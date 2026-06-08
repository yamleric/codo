package store

import (
	"strings"
	"testing"
	"time"
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

func TestDailyReportRecipientFallsBackToUsernameEmail(t *testing.T) {
	if got := DailyReportRecipient(DailyReport{}, "owner@example.com"); got != "owner@example.com" {
		t.Fatalf("recipient = %q, want username email", got)
	}
}

func TestDailyReportRecipientPrefersExplicitEmail(t *testing.T) {
	report := DailyReport{Email: "summary@example.com"}
	if got := DailyReportRecipient(report, "owner@example.com"); got != "summary@example.com" {
		t.Fatalf("recipient = %q, want explicit email", got)
	}
}

func TestInitialNotifyChannelUsesEmailForEmailUsername(t *testing.T) {
	if got := initialNotifyChannel("owner@example.com"); got != "email" {
		t.Fatalf("initial notify channel = %q, want email", got)
	}
}

func TestInitialNotifyChannelDefaultsToTelegramOtherwise(t *testing.T) {
	if got := initialNotifyChannel("owner"); got != "telegram" {
		t.Fatalf("initial notify channel = %q, want telegram", got)
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

func TestNormalizeFeedbackInputRejectsUnknownRating(t *testing.T) {
	input := normalizeFeedbackInput(FeedbackInput{
		TargetType: " Article ",
		TargetID:   " a-1 ",
		Rating:     "unknown",
		Source:     "",
	})
	if input.TargetType != "article" || input.TargetID != "a-1" {
		t.Fatalf("target not normalized: %#v", input)
	}
	if input.Rating != "" {
		t.Fatalf("rating = %q, want empty", input.Rating)
	}
	if input.Source != "manual" {
		t.Fatalf("source = %q, want manual", input.Source)
	}
}

func TestFeedbackMemoryMappingKeepsNotifyAndSilentDistinct(t *testing.T) {
	notifyType, _ := feedbackMemoryMapping(feedbackNotifySimilar)
	silentType, _ := feedbackMemoryMapping(feedbackSilentSimilar)
	if notifyType != memoryNotify {
		t.Fatalf("notify type = %q, want %q", notifyType, memoryNotify)
	}
	if silentType != memorySilent {
		t.Fatalf("silent type = %q, want %q", silentType, memorySilent)
	}
}

func TestProfileFromRawNormalizesLists(t *testing.T) {
	profile, err := profileFromRaw("demo-user", true, `{"recent_intents":["  AI 产品落地  "],"feedback_count":2,"memory_count":3}`, 4, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !profile.MemoryEnabled || profile.Version != 4 {
		t.Fatalf("unexpected profile flags: %#v", profile)
	}
	if len(profile.RecentIntents) != 1 || profile.RecentIntents[0] != "AI 产品落地" {
		t.Fatalf("recent intents not normalized: %#v", profile.RecentIntents)
	}
	if profile.FeedbackCount != 2 || profile.MemoryCount != 3 {
		t.Fatalf("counts not preserved: %#v", profile)
	}
}
