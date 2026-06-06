package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codo/codo/internal/domain/task"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	cfg    Config
	http   *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llm: http %d: %s", resp.StatusCode, raw)
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("llm: parse response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices")
	}
	return cr.Choices[0].Message.Content, nil
}

func (c *Client) Filter(ctx context.Context, _ string, content string) (task.FilterDecision, string, error) {
	type result struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}

	out, err := c.complete(ctx,
		`你是内容过滤器。只输出 JSON，不要其他文字：{"decision":"pass|silent|discard","reason":"一句话原因"}
- pass：高质量有价值，保存并通知
- silent：一般内容，静默保存
- discard：低质量或广告，丢弃`,
		truncate(content, 2000),
	)
	if err != nil {
		return "", "", fmt.Errorf("filter: %w", err)
	}

	out = extractJSON(out)
	var r result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return task.FilterSilent, "parse error", nil
	}
	d := task.FilterDecision(strings.TrimSpace(strings.ToLower(r.Decision)))
	if d != task.FilterPass && d != task.FilterSilent && d != task.FilterDiscard {
		d = task.FilterSilent
	}
	return d, r.Reason, nil
}

func (c *Client) Summarize(ctx context.Context, _ *task.Task, content string) (string, error) {
	out, err := c.complete(ctx,
		"用中文总结以下内容，300字以内，突出核心观点和对读者的价值。",
		truncate(content, 6000),
	)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return out, nil
}

func (c *Client) Classify(ctx context.Context, content string) (string, error) {
	type result struct {
		Category string `json:"category"`
	}

	out, err := c.complete(ctx,
		`对邮件分类，只输出 JSON，不要其他文字：{"category":"important|notify|spam"}`,
		truncate(content, 2000),
	)
	if err != nil {
		return "", fmt.Errorf("classify: %w", err)
	}

	out = extractJSON(out)
	var r result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return "notify", nil
	}
	return strings.TrimSpace(strings.ToLower(r.Category)), nil
}

func (c *Client) Extract(ctx context.Context, messages string) (string, error) {
	out, err := c.complete(ctx,
		"从群聊消息中提取关键信息：重要通知、决策、文件链接。过滤闲聊。用中文简洁输出。",
		truncate(messages, 4000),
	)
	if err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	return out, nil
}

// extractJSON finds the first {...} block in s, handling model preamble text.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}
