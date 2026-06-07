package main

import (
	"testing"

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

func TestApplySettingsPatchUpdatesDailyReport(t *testing.T) {
	email := "me@example.com"
	enabled := true
	hour := 8
	timezone := "Asia/Shanghai"
	maxItems := 12

	updated, err := applySettingsPatch(storeDefaultSettings(), settingsPatch{
		DailyReport: &dailyReportPatch{
			Enabled:  &enabled,
			Email:    &email,
			Hour:     &hour,
			Timezone: &timezone,
			MaxItems: &maxItems,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.DailyReport.Enabled ||
		updated.DailyReport.Email != email ||
		updated.DailyReport.Hour != hour ||
		updated.DailyReport.Timezone != timezone ||
		updated.DailyReport.MaxItems != maxItems {
		t.Fatalf("daily report patch not applied: %#v", updated.DailyReport)
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
