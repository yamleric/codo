package store

import (
	"strings"
	"testing"
)

func TestDailyReportFromRawPolicyUsesDefaultsWhenMissing(t *testing.T) {
	report := dailyReportFromRawPolicy(`{"summary_style":"concise"}`)
	if report.Hour != 21 || report.Timezone != "Asia/Shanghai" || report.MaxItems != 20 || report.Enabled {
		t.Fatalf("unexpected defaults: %#v", report)
	}
}

func TestDailyReportFromRawPolicyKeepsExplicitMidnightHour(t *testing.T) {
	report := dailyReportFromRawPolicy(`{"daily_report":{"enabled":true,"email":"me@example.com","hour":0,"timezone":"Asia/Shanghai","max_items":10}}`)
	report = NormalizeDailyReport(report)
	if !report.Enabled || report.Hour != 0 || report.Email != "me@example.com" || report.MaxItems != 10 {
		t.Fatalf("unexpected explicit report config: %#v", report)
	}
}

func TestBuildArticleChunksIncludesSummary(t *testing.T) {
	chunks := BuildArticleChunks("核心摘要", "正文内容")
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "核心摘要") || !strings.Contains(chunks[0].Content, "正文内容") {
		t.Fatalf("chunk content missing summary or body: %q", chunks[0].Content)
	}
	if chunks[0].TokenEstimate <= 0 {
		t.Fatalf("token estimate not set: %#v", chunks[0])
	}
}

func TestBuildArticleChunksSplitsLongContent(t *testing.T) {
	chunks := BuildArticleChunks("", strings.Repeat("长正文", 1200))
	if len(chunks) < 2 {
		t.Fatalf("len(chunks) = %d, want multiple chunks", len(chunks))
	}
	if chunks[0].Content == chunks[1].Content {
		t.Fatalf("chunks should not be identical")
	}
}
