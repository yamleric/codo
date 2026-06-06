package task

import (
	"encoding/json"
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
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
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
