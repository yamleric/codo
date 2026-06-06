package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"

	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}
var sanitizer = bluemonday.StrictPolicy()
var whitespace = regexp.MustCompile(`\s+`)

const wechatParserReference = "https://github.com/huanjuedadehen/wechat-article-parser"

// HTTPFetcher implements pipeline.Fetcher using plain HTTP + readability.
type HTTPFetcher struct{}

func NewHTTP() *HTTPFetcher { return &HTTPFetcher{} }

func (f *HTTPFetcher) Fetch(ctx context.Context, t *task.Task) (string, error) {
	if t.URL == "" {
		if t.RawContent() != "" {
			return t.RawContent(), nil
		}
		return "", fmt.Errorf("fetcher: no URL or raw content")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return "", fmt.Errorf("fetcher: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Codo/1.0)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetcher: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetcher: http %d for %s", resp.StatusCode, t.URL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fetcher: read body: %w", err)
	}

	pageURL, _ := url.Parse(t.URL)
	if pageURL.Hostname() == "mp.weixin.qq.com" {
		text, err := extractWechatArticle(body)
		if err != nil {
			return "", fmt.Errorf("fetcher: wechat article: %w", err)
		}
		return validateContent(text)
	}

	parser := readability.NewParser()
	article, err := parser.Parse(bytes.NewReader(body), pageURL)
	if err != nil {
		return "", fmt.Errorf("fetcher: readability: %w", err)
	}

	var buf bytes.Buffer
	if err := article.RenderText(&buf); err != nil {
		return "", fmt.Errorf("fetcher: render text: %w", err)
	}

	return validateContent(buf.String())
}

func extractWechatArticle(body []byte) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	// Verification detection and alternate article containers are informed by
	// wechat-article-parser (MIT); this implementation remains native Go.
	// See CREDITS.md and wechatParserReference.
	if doc.Find(`script[src*="secitptpage/template/verify.js"]`).Length() > 0 {
		return "", fmt.Errorf("verification required")
	}

	content := doc.Find("#js_content, .rich_media_content, .original_page").First()
	if content.Length() == 0 {
		return "", fmt.Errorf("missing article content")
	}
	content.Find("script, style, noscript").Remove()

	title := cleanText(doc.Find("#activity-name").First().Text())
	account := cleanText(doc.Find("#js_name").First().Text())
	paragraphs := make([]string, 0, 32)
	content.Find("p, li, blockquote, h1, h2, h3, h4, h5, h6").Each(func(_ int, selection *goquery.Selection) {
		text := cleanText(selection.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})
	if len(paragraphs) == 0 {
		paragraphs = append(paragraphs, cleanText(content.Text()))
	}

	parts := make([]string, 0, len(paragraphs)+2)
	if title != "" {
		parts = append(parts, "标题："+title)
	}
	if account != "" {
		parts = append(parts, "来源："+account)
	}
	parts = append(parts, paragraphs...)
	return strings.Join(parts, "\n\n"), nil
}

func validateContent(text string) (string, error) {
	text = sanitizer.Sanitize(text)
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 100 {
		return "", fmt.Errorf("fetcher: content too short (%d chars)", len([]rune(text)))
	}
	return text, nil
}

func cleanText(text string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(text, " "))
}

// Verify at compile time.
var _ pipeline.Fetcher = (*HTTPFetcher)(nil)
