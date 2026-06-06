package task

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotJSONMatchesDashboardContract(t *testing.T) {
	item := New("task-1", "user-1", SourceRSS, ContentWebPage, "https://example.com", "")
	item.SetStatus(StatusAnalyzing)
	item.SetFilterDecision(FilterPass)
	item.AddStep(Step{
		Label:    "抓取网页",
		Status:   StepOK,
		Detail:   "1200 字",
		Duration: 1250 * time.Millisecond,
	})

	data, err := json.Marshal(item.Snapshot())
	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		ID        string   `json:"id"`
		Status    string   `json:"status"`
		Category  string   `json:"category"`
		Tags      []string `json:"tags"`
		CreatedAt string   `json:"created_at"`
		Steps     []struct {
			Label      string `json:"label"`
			DurationMs int64  `json:"duration_ms"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	if payload.ID != "task-1" || payload.Status != "analyzing" || payload.CreatedAt == "" {
		t.Fatalf("unexpected snapshot payload: %+v", payload)
	}
	if len(payload.Steps) != 1 || payload.Steps[0].Label != "抓取网页" || payload.Steps[0].DurationMs != 1250 {
		t.Fatalf("unexpected snapshot steps: %+v", payload.Steps)
	}
}

func TestClassificationIsNormalized(t *testing.T) {
	got := NormalizeClassification(Classification{
		Category: "unknown",
		Tags:     []string{" AI ", "AI", "", "一个特别特别特别长的标签"},
		Reason:   strings.Repeat("原因", 60),
	})

	if got.Category != "其他" {
		t.Fatalf("unexpected category: %q", got.Category)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "AI" {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
	if len([]rune(got.Tags[1])) > 16 {
		t.Fatalf("tag was not truncated: %q", got.Tags[1])
	}
	if len([]rune(got.Reason)) > 80 {
		t.Fatalf("reason was not truncated: %q", got.Reason)
	}
}

func TestSnapshotIncludesClassification(t *testing.T) {
	item := New("task-1", "user-1", SourceRSS, ContentWebPage, "https://example.com", "")
	item.SetClassification("技术", []string{"Go", "并发"})

	snap := item.Snapshot()
	if snap.Category != "技术" {
		t.Fatalf("unexpected category: %q", snap.Category)
	}
	if len(snap.Tags) != 2 || snap.Tags[0] != "Go" || snap.Tags[1] != "并发" {
		t.Fatalf("unexpected tags: %#v", snap.Tags)
	}
}

func TestRawContentCanBeUpdatedAfterFetch(t *testing.T) {
	item := New("task-1", "user-1", SourceManual, ContentWebPage, "https://example.com", "")
	item.SetRawContent("fetched article content")

	if got := item.RawContent(); got != "fetched article content" {
		t.Fatalf("unexpected raw content: %q", got)
	}
}
