package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/codo/codo/internal/domain/task"
)

func TestWebPageManualSubmissionSkipsDiscardFilter(t *testing.T) {
	st := &fakePipelineStore{}
	filterer := &fakeFilterer{decision: task.FilterDiscard, reason: "would discard"}
	notifier := &fakeNotifier{}
	p := NewWebPage(&fakeFetcher{content: "user selected article"}, filterer, &fakeSummarizer{summary: "summary"}, st, notifier, nil)
	item := task.New("task-1", "user-1", task.SourceManual, task.ContentWebPage, "https://example.com/article", "")

	if err := p.Run(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if filterer.calls != 0 {
		t.Fatalf("filter called %d times, want 0", filterer.calls)
	}
	if item.FilterDecision() != task.FilterPass {
		t.Fatalf("decision = %q, want pass", item.FilterDecision())
	}
	if item.Status() != task.StatusDone {
		t.Fatalf("status = %q, want done", item.Status())
	}
	if st.knowledgeSaves != 1 {
		t.Fatalf("knowledge saves = %d, want 1", st.knowledgeSaves)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", notifier.calls)
	}
}

func TestWebPageBookmarkSkipsDiscardFilterAndStaysSilent(t *testing.T) {
	st := &fakePipelineStore{}
	filterer := &fakeFilterer{decision: task.FilterDiscard, reason: "would discard"}
	notifier := &fakeNotifier{}
	p := NewWebPage(&fakeFetcher{content: "bookmarked article"}, filterer, &fakeSummarizer{summary: "summary"}, st, notifier, nil)
	item := task.New("task-2", "user-1", task.SourceBookmark, task.ContentWebPage, "https://example.com/bookmark", "")

	if err := p.Run(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if filterer.calls != 0 {
		t.Fatalf("filter called %d times, want 0", filterer.calls)
	}
	if item.FilterDecision() != task.FilterSilent {
		t.Fatalf("decision = %q, want silent", item.FilterDecision())
	}
	if item.Status() != task.StatusDone {
		t.Fatalf("status = %q, want done", item.Status())
	}
	if st.knowledgeSaves != 1 {
		t.Fatalf("knowledge saves = %d, want 1", st.knowledgeSaves)
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
}

func TestWebPageAutoSourceStillUsesDiscardFilter(t *testing.T) {
	st := &fakePipelineStore{}
	filterer := &fakeFilterer{decision: task.FilterDiscard, reason: "low value"}
	notifier := &fakeNotifier{}
	p := NewWebPage(&fakeFetcher{content: "auto fetched article"}, filterer, &fakeSummarizer{summary: "summary"}, st, notifier, nil)
	item := task.New("task-3", "user-1", task.SourceRSS, task.ContentWebPage, "https://example.com/rss", "")

	if err := p.Run(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if filterer.calls != 1 {
		t.Fatalf("filter called %d times, want 1", filterer.calls)
	}
	if item.FilterDecision() != task.FilterDiscard {
		t.Fatalf("decision = %q, want discard", item.FilterDecision())
	}
	if item.Status() != task.StatusDiscarded {
		t.Fatalf("status = %q, want discarded", item.Status())
	}
	if st.knowledgeSaves != 0 {
		t.Fatalf("knowledge saves = %d, want 0", st.knowledgeSaves)
	}
}

func TestExplicitSourceDecisionForLinuxDoIsSilent(t *testing.T) {
	decision, _, ok := explicitSourceDecision(task.SourceLinuxDo)
	if !ok || decision != task.FilterSilent {
		t.Fatalf("linux.do decision = %q ok=%v, want silent true", decision, ok)
	}
}

func TestWebPageTranslationFeedsSummaryAndMetadata(t *testing.T) {
	st := &fakePipelineStore{}
	filterer := &fakeFilterer{decision: task.FilterSilent, reason: "save"}
	summarizer := &fakeTranslatingSummarizer{
		fakeSummarizer: fakeSummarizer{summary: "摘要"},
		translation: task.Translation{
			Status:          "translated",
			SourceLang:      "en",
			TargetLang:      "zh-CN",
			Content:         "中文译文正文",
			Scope:           "summary_knowledge",
			TranslatedChars: 6,
		},
	}
	p := NewWebPage(&fakeFetcher{content: "English article body with enough content to translate."}, filterer, summarizer, st, &fakeNotifier{}, nil)
	item := task.New("task-4", "user-1", task.SourceRSS, task.ContentWebPage, "https://example.com/en", "")

	if err := p.Run(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if summarizer.lastContent != "中文译文正文" {
		t.Fatalf("summary content = %q, want translated content", summarizer.lastContent)
	}
	if _, ok := item.Metadata()["translation"]; !ok {
		t.Fatalf("translation metadata missing: %#v", item.Metadata())
	}
}

type fakeFetcher struct {
	content string
	err     error
}

func (f *fakeFetcher) Fetch(_ context.Context, _ *task.Task) (string, error) {
	return f.content, f.err
}

type fakeFilterer struct {
	decision task.FilterDecision
	reason   string
	err      error
	calls    int
}

func (f *fakeFilterer) Filter(_ context.Context, _, _ string) (task.FilterDecision, string, error) {
	f.calls++
	return f.decision, f.reason, f.err
}

type fakeSummarizer struct {
	summary     string
	err         error
	calls       int
	lastContent string
}

func (f *fakeSummarizer) Summarize(_ context.Context, _ *task.Task, content string) (string, error) {
	f.calls++
	f.lastContent = content
	return f.summary, f.err
}

type fakeTranslatingSummarizer struct {
	fakeSummarizer
	translation task.Translation
}

func (f *fakeTranslatingSummarizer) Translate(_ context.Context, _, _ string) (task.Translation, error) {
	return f.translation, nil
}

type fakePipelineStore struct {
	duplicate      bool
	knowledgeSaves int
	steps          []task.Step
	states         []task.Status
}

func (s *fakePipelineStore) SaveTaskState(_ context.Context, t *task.Task) error {
	s.states = append(s.states, t.Status())
	return nil
}

func (s *fakePipelineStore) SaveKnowledgeItem(_ context.Context, _ *task.Task) error {
	s.knowledgeSaves++
	return nil
}

func (s *fakePipelineStore) AppendStep(_ context.Context, _ string, step task.Step) error {
	s.steps = append(s.steps, step)
	return nil
}

func (s *fakePipelineStore) IsDuplicate(_ context.Context, _ string) (bool, error) {
	return s.duplicate, nil
}

type fakeNotifier struct {
	calls int
	err   error
}

func (n *fakeNotifier) Send(_ context.Context, _ string, message string) error {
	n.calls++
	if message == "" {
		return fmt.Errorf("empty message")
	}
	return n.err
}
