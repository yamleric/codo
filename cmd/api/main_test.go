package main

import (
	"testing"

	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/store"
)

func TestBookmarkInputsFromPayloadExtractsAndDeduplicatesURLs(t *testing.T) {
	title := "显式标题"
	folder := "产品"
	note := "稍后读"
	url := "https://example.com/article"
	inputs := bookmarkInputsFromPayload(bookmarkImportPayload{
		URL:    "分享文本 https://example.com/article ",
		Text:   "https://example.com/article\nhttps://example.com/second，",
		Folder: "待读",
		Bookmarks: []bookmarkPayload{
			{URL: &url, Title: &title, Folder: &folder, Note: &note},
		},
	})

	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2: %#v", len(inputs), inputs)
	}
	if inputs[0].URL != "https://example.com/article" || inputs[0].Folder != "待读" {
		t.Fatalf("unexpected first input: %#v", inputs[0])
	}
	if inputs[1].URL != "https://example.com/second" || inputs[1].Folder != "待读" {
		t.Fatalf("unexpected second input: %#v", inputs[1])
	}
}

func TestBookmarkInputsFromPayloadKeepsExplicitMetadata(t *testing.T) {
	title := "文章"
	folder := "技术"
	note := "重点"
	url := "https://example.com/a"
	inputs := bookmarkInputsFromPayload(bookmarkImportPayload{
		Bookmarks: []bookmarkPayload{
			{URL: &url, Title: &title, Folder: &folder, Note: &note},
		},
	})

	if len(inputs) != 1 {
		t.Fatalf("len(inputs) = %d, want 1", len(inputs))
	}
	if inputs[0].Title != title || inputs[0].Folder != folder || inputs[0].Note != note {
		t.Fatalf("metadata not preserved: %#v", inputs[0])
	}
}

func TestLinuxDoBookmarkInputsFromCSVFiltersAndDeduplicates(t *testing.T) {
	csvData := []byte(`bookmarkable_id,bookmarkable_type,link,name,created_at
1,Post,https://linux.do/t/topic/123/4,,2026-06-01 10:00:00
2,Topic,https://linux.do/t/topic/123/4,,2026-06-01 10:01:00
3,Post,https://meta.linux.do/t/topic/456,Meta topic,2026-06-01 10:02:00
4,Post,https://example.com/t/topic/999,外部链接,2026-06-01 10:03:00
5,Post,,空链接,2026-06-01 10:04:00
`)

	inputs, parsed, ignored, err := linuxDoBookmarkInputsFromCSV(csvData)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != 5 || ignored != 3 {
		t.Fatalf("parsed=%d ignored=%d, want parsed=5 ignored=3", parsed, ignored)
	}
	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2: %#v", len(inputs), inputs)
	}
	if inputs[0].URL != "https://linux.do/t/topic/123/4" || inputs[0].Folder != "linux.do" {
		t.Fatalf("unexpected first input: %#v", inputs[0])
	}
	if inputs[1].Title != "Meta topic" || inputs[1].URL != "https://meta.linux.do/t/topic/456" {
		t.Fatalf("unexpected second input: %#v", inputs[1])
	}
}

func TestBookmarkTaskSourceDetectsLinuxDo(t *testing.T) {
	source := bookmarkTaskSource(store.BookmarkRow{URL: "https://linux.do/t/topic/123"})
	if source != task.SourceLinuxDo {
		t.Fatalf("source = %q, want %q", source, task.SourceLinuxDo)
	}
	source = bookmarkTaskSource(store.BookmarkRow{URL: "https://example.com/article"})
	if source != task.SourceBookmark {
		t.Fatalf("source = %q, want %q", source, task.SourceBookmark)
	}
}

func TestApplySettingsPatchUpdatesDailyReport(t *testing.T) {
	email := "me@example.com"
	enabled := true
	hour := 8
	timezone := "Asia/Shanghai"
	maxItems := 12
	frequency := "weekly"
	channels := []string{"email", "telegram"}
	sources := []string{"rss", "linux_do"}
	categories := []string{"AI", "产品"}
	categoryMode := "include"
	splitByCategory := true

	updated, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		DailyReport: &dailyReportPatch{
			Enabled:         &enabled,
			Email:           &email,
			Hour:            &hour,
			Timezone:        &timezone,
			MaxItems:        &maxItems,
			Frequency:       &frequency,
			Channels:        &channels,
			Sources:         &sources,
			Categories:      &categories,
			CategoryMode:    &categoryMode,
			SplitByCategory: &splitByCategory,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.DailyReport.Enabled ||
		updated.DailyReport.Email != email ||
		updated.DailyReport.Hour != hour ||
		updated.DailyReport.Timezone != timezone ||
		updated.DailyReport.MaxItems != maxItems ||
		updated.DailyReport.Frequency != frequency ||
		len(updated.DailyReport.Channels) != 2 ||
		len(updated.DailyReport.Sources) != 2 ||
		updated.DailyReport.CategoryMode != categoryMode ||
		len(updated.DailyReport.Categories) != 2 ||
		!updated.DailyReport.SplitByCategory {
		t.Fatalf("daily report patch not applied: %#v", updated.DailyReport)
	}
}

func TestApplySettingsPatchAllowsTelegramOnlyDailyReportWithoutEmail(t *testing.T) {
	enabled := true
	channels := []string{"telegram"}
	updated, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		DailyReport: &dailyReportPatch{Enabled: &enabled, Channels: &channels},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.DailyReport.Enabled || len(updated.DailyReport.Channels) != 1 || updated.DailyReport.Channels[0] != "telegram" {
		t.Fatalf("expected telegram-only daily report, got %#v", updated.DailyReport)
	}
}

func TestApplySettingsPatchRejectsInvalidDailyReportEmail(t *testing.T) {
	email := "not an email"
	_, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		DailyReport: &dailyReportPatch{Email: &email},
	})
	if err == nil {
		t.Fatal("expected invalid email error")
	}
}

func TestApplySettingsPatchAllowsMissingDailyReportEmailWhenUsernameIsEmail(t *testing.T) {
	enabled := true
	updated, err := applySettingsPatch(store.UserSettings{
		UserID:         "demo-user",
		Username:       "owner@example.com",
		NotifyChannel:  "telegram",
		FilterKeywords: []string{},
		ModelPolicy:    store.DefaultUserModelPolicy(),
		DailyReport:    store.DefaultDailyReport(),
	}, settingsPatch{
		DailyReport: &dailyReportPatch{Enabled: &enabled},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.DailyReport.Enabled || updated.DailyReport.Email != "" {
		t.Fatalf("expected enabled daily report with blank email, got %#v", updated.DailyReport)
	}
}

func TestApplySettingsPatchAcceptsEmailNotifyChannel(t *testing.T) {
	channel := "email"
	updated, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{NotifyChannel: &channel})
	if err != nil {
		t.Fatal(err)
	}
	if updated.NotifyChannel != "email" {
		t.Fatalf("notify channel = %q, want email", updated.NotifyChannel)
	}
}

func TestApplySettingsPatchUpdatesTranslationPolicy(t *testing.T) {
	enabled := true
	mode := "english_only"
	target := "zh-CN"
	scope := "knowledge"
	maxChars := 12000
	updated, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		Translation: &translationPatch{
			Enabled:        &enabled,
			Mode:           &mode,
			TargetLanguage: &target,
			Scope:          &scope,
			MaxChars:       &maxChars,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := updated.ModelPolicy.Translation
	if !policy.Enabled || policy.Mode != mode || policy.TargetLanguage != target || policy.Scope != scope || policy.MaxChars != maxChars {
		t.Fatalf("translation patch not applied: %#v", policy)
	}
}

func TestApplySettingsPatchRejectsInvalidTranslationScope(t *testing.T) {
	scope := "everything"
	_, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		Translation: &translationPatch{Scope: &scope},
	})
	if err == nil {
		t.Fatal("expected invalid translation scope error")
	}
}

func storeDefaultSettings() store.UserSettings {
	return store.UserSettings{
		UserID:         "demo-user",
		Username:       "owner",
		NotifyChannel:  "telegram",
		FilterKeywords: []string{},
		ModelPolicy:    store.DefaultUserModelPolicy(),
		DailyReport:    store.DefaultDailyReport(),
	}
}
