package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codo/codo/internal/infra/store"
)

func TestAnswerUsesSearchResultsAsCitations(t *testing.T) {
	st := &fakeStore{keywordRows: []store.SearchChunkRow{
		{
			ChunkID:     "chunk-1",
			ArticleID:   "article-1",
			Title:       "政策分析",
			URL:         "https://example.com/policy",
			Source:      "manual",
			ContentType: "webpage",
			Summary:     "摘要",
			Category:    "政治",
			Tags:        []string{"选举"},
			Content:     "这是一段关于政治议题的知识库正文，用于回答用户的问题。",
			Score:       1,
			CreatedAt:   time.Now(),
		},
	}}
	llm := &fakeCompleter{configured: true, answer: "可以依据政策分析回答。[1]"}
	service := NewService(st, llm, nil)

	resp, err := service.Answer(context.Background(), "demo-user", "政治议题是什么")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answer != "可以依据政策分析回答。[1]" {
		t.Fatalf("unexpected answer: %q", resp.Answer)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].Index != 1 || resp.Citations[0].Category != "政治" {
		t.Fatalf("unexpected citations: %#v", resp.Citations)
	}
	if !strings.Contains(llm.userPrompt, "[1] 政策分析") {
		t.Fatalf("prompt missing citation: %s", llm.userPrompt)
	}
}

func TestAnswerRequiresConfiguredLLMWhenResultsExist(t *testing.T) {
	st := &fakeStore{keywordRows: []store.SearchChunkRow{
		{ChunkID: "chunk-1", ArticleID: "article-1", Content: "正文", Score: 1, CreatedAt: time.Now()},
	}}
	service := NewService(st, &fakeCompleter{}, nil)

	_, err := service.Answer(context.Background(), "demo-user", "问题")
	if !errors.Is(err, ErrLLMNotConfigured) {
		t.Fatalf("err = %v, want ErrLLMNotConfigured", err)
	}
}

func TestAnswerReturnsEmptyKnowledgeMessageWithoutResults(t *testing.T) {
	service := NewService(&fakeStore{}, nil, nil)

	resp, err := service.Answer(context.Background(), "demo-user", "不存在的问题")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Citations) != 0 || !strings.Contains(resp.Answer, "没有找到") {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestSearchExpandsNaturalLanguageQuestion(t *testing.T) {
	st := &fakeStore{keywordRowsByQuery: map[string][]store.SearchChunkRow{
		"政治议题": {
			{
				ChunkID:   "chunk-1",
				ArticleID: "article-1",
				Content:   "这是一段关于政治议题的知识库正文。",
				Score:     1,
				CreatedAt: time.Now(),
			},
		},
	}}
	service := NewService(st, nil, nil)

	resp, err := service.Search(context.Background(), "demo-user", "政治议题是什么", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results = %d, want 1; queries = %#v", len(resp.Results), st.queries)
	}
	if !containsString(st.queries, "政治议题") {
		t.Fatalf("expanded queries missing cleaned term: %#v", st.queries)
	}
}

func TestAnswerPromptUsesFullChunkContext(t *testing.T) {
	content := strings.Repeat("背景信息。", 80) + "关键依据：日报需要按分类拆分发送。"
	st := &fakeStore{keywordRows: []store.SearchChunkRow{
		{
			ChunkID:   "chunk-1",
			ArticleID: "article-1",
			Title:     "日报设置",
			Content:   content,
			Score:     1,
			CreatedAt: time.Now(),
		},
	}}
	llm := &fakeCompleter{configured: true, answer: "日报可以按分类拆分发送。[1]"}
	service := NewService(st, llm, nil)

	_, err := service.Answer(context.Background(), "demo-user", "日报怎么发")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llm.userPrompt, "关键依据：日报需要按分类拆分发送") {
		t.Fatalf("prompt missing full chunk context: %s", llm.userPrompt)
	}
}

type fakeStore struct {
	keywordRows        []store.SearchChunkRow
	keywordRowsByQuery map[string][]store.SearchChunkRow
	vectorRows         []store.SearchChunkRow
	chunks             []store.ChunkEmbeddingRow
	queries            []string
}

func (f *fakeStore) SearchArticleChunksKeyword(_ context.Context, _, query string, _ int) ([]store.SearchChunkRow, error) {
	f.queries = append(f.queries, query)
	if f.keywordRowsByQuery != nil {
		if rows, ok := f.keywordRowsByQuery[query]; ok {
			return rows, nil
		}
		return []store.SearchChunkRow{}, nil
	}
	return f.keywordRows, nil
}

func (f *fakeStore) SearchArticleChunksVector(context.Context, string, []float32, int) ([]store.SearchChunkRow, error) {
	return f.vectorRows, nil
}

func (f *fakeStore) ListChunksNeedingEmbedding(context.Context, string, int) ([]store.ChunkEmbeddingRow, error) {
	return f.chunks, nil
}

func (f *fakeStore) UpdateChunkEmbedding(context.Context, string, string, []float32) error {
	return nil
}

type fakeCompleter struct {
	configured   bool
	answer       string
	systemPrompt string
	userPrompt   string
}

func (f *fakeCompleter) Configured() bool {
	return f.configured
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string) (string, error) {
	f.systemPrompt = system
	f.userPrompt = user
	return f.answer, nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
