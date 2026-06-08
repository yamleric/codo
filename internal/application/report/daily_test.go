package report

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codo/codo/internal/infra/store"
)

func TestBuildDailyEmailBodyGroupsByCategory(t *testing.T) {
	body := BuildDailyEmailBody("2026-06-07", "Asia/Shanghai", []store.DailyArticleRow{
		{Source: "rss", Summary: "第一条摘要\n后续细节", Category: "AI", Tags: []string{"模型"}, URL: "https://example.com/a"},
		{Source: "bookmark", Summary: "第二条摘要", Category: "", URL: "https://example.com/b"},
	})

	for _, expected := range []string{
		"Codo 日报",
		"日期：2026-06-07",
		"## AI",
		"第一条摘要",
		"标签：模型",
		"## 未分类",
		"收藏夹",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in body:\n%s", expected, body)
		}
	}
}

func TestRunForUserSendsOnceAfterConfiguredHour(t *testing.T) {
	st := &fakeStore{
		settings: store.UserSettings{
			UserID: "demo-user",
			DailyReport: store.DailyReport{
				Enabled:  true,
				Email:    "me@example.com",
				Hour:     9,
				Timezone: "Asia/Shanghai",
				MaxItems: 10,
				Channels: []string{"email"},
			},
		},
		articles: []store.DailyArticleRow{
			{Source: "rss", Summary: "摘要", Category: "技术"},
		},
	}
	email := &fakeEmail{}
	service := NewService(st, email)

	result, err := service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 7, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.ItemCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if email.sends != 1 || st.sent != 1 {
		t.Fatalf("expected one send and one sent mark, got sends=%d sent=%d", email.sends, st.sent)
	}

	result, err = service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 7, 11, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "already_done" {
		t.Fatalf("expected already_done, got %#v", result)
	}
	if email.sends != 1 {
		t.Fatalf("expected no duplicate send, got sends=%d", email.sends)
	}
}

func TestRunForUserFallsBackToUsernameEmail(t *testing.T) {
	st := &fakeStore{
		settings: store.UserSettings{
			UserID:   "demo-user",
			Username: "owner@example.com",
			DailyReport: store.DailyReport{
				Enabled:  true,
				Email:    "",
				Hour:     9,
				Timezone: "Asia/Shanghai",
				MaxItems: 10,
				Channels: []string{"email"},
			},
		},
		articles: []store.DailyArticleRow{
			{Source: "rss", Summary: "摘要", Category: "技术"},
		},
	}
	email := &fakeEmail{}
	service := NewService(st, email)

	result, err := service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 7, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.ItemCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(email.recipients) != 1 || email.recipients[0] != "owner@example.com" {
		t.Fatalf("unexpected recipients: %#v", email.recipients)
	}
}

func TestRunForUserSendsWeeklyTelegramWithFilters(t *testing.T) {
	st := &fakeStore{
		settings: store.UserSettings{
			UserID: "demo-user",
			DailyReport: store.DailyReport{
				Enabled:      true,
				Hour:         9,
				Timezone:     "Asia/Shanghai",
				MaxItems:     10,
				Frequency:    "weekly",
				Channels:     []string{"telegram"},
				Sources:      []string{"rss"},
				Categories:   []string{"AI"},
				CategoryMode: "include",
			},
		},
		articles: []store.DailyArticleRow{
			{Source: "rss", Summary: "摘要", Category: "AI"},
		},
	}
	messenger := &fakeMessenger{}
	service := NewService(st, nil, messenger)

	result, err := service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 8, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || result.ReportDate != "2026-06-01" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if messenger.sends != 1 || !strings.Contains(messenger.messages[0], "Codo 周报") {
		t.Fatalf("expected weekly telegram send, got %#v", messenger.messages)
	}
	if st.query.Limit != 10 || st.query.CategoryMode != "include" || len(st.query.Sources) != 1 || st.query.Sources[0] != "rss" {
		t.Fatalf("query not passed through: %#v", st.query)
	}
}

func TestRunForUserWaitsForWeeklySchedule(t *testing.T) {
	st := &fakeStore{
		settings: store.UserSettings{
			UserID: "demo-user",
			DailyReport: store.DailyReport{
				Enabled:   true,
				Hour:      9,
				Timezone:  "Asia/Shanghai",
				Frequency: "weekly",
				Channels:  []string{"telegram"},
			},
		},
	}
	service := NewService(st, nil, &fakeMessenger{})

	result, err := service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 9, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" || st.started {
		t.Fatalf("expected pending without starting report, got result=%#v started=%v", result, st.started)
	}
}

func TestRunForUserSplitsByCategory(t *testing.T) {
	st := &fakeStore{
		settings: store.UserSettings{
			UserID: "demo-user",
			DailyReport: store.DailyReport{
				Enabled:         true,
				Email:           "me@example.com",
				Hour:            9,
				Timezone:        "Asia/Shanghai",
				MaxItems:        10,
				Channels:        []string{"email"},
				SplitByCategory: true,
			},
		},
		articles: []store.DailyArticleRow{
			{Source: "rss", Summary: "AI 摘要", Category: "AI"},
			{Source: "rss", Summary: "产品摘要", Category: "产品"},
		},
	}
	email := &fakeEmail{}
	service := NewService(st, email)

	result, err := service.RunForUser(context.Background(), "demo-user", time.Date(2026, 6, 7, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "sent" || email.sends != 2 {
		t.Fatalf("expected split sends, got result=%#v sends=%d", result, email.sends)
	}
}

type fakeStore struct {
	settings store.UserSettings
	articles []store.DailyArticleRow
	query    store.DailyArticleQuery
	started  bool
	sent     int
}

func (f *fakeStore) GetUserSettings(context.Context, string) (store.UserSettings, error) {
	return f.settings, nil
}

func (f *fakeStore) ListDailyArticles(_ context.Context, _ string, _, _ time.Time, query store.DailyArticleQuery) ([]store.DailyArticleRow, error) {
	f.query = query
	return f.articles, nil
}

func (f *fakeStore) StartDailyReport(context.Context, string, string) (bool, error) {
	if f.started {
		return false, nil
	}
	f.started = true
	return true, nil
}

func (f *fakeStore) MarkDailyReportSent(context.Context, string, string, int) error {
	f.sent++
	return nil
}

func (f *fakeStore) MarkDailyReportSkipped(context.Context, string, string, int) error {
	return nil
}

func (f *fakeStore) MarkDailyReportFailed(context.Context, string, string, error) error {
	return nil
}

type fakeEmail struct {
	sends      int
	recipients []string
}

func (f *fakeEmail) Send(_ context.Context, recipients []string, _, _ string) error {
	f.sends++
	f.recipients = append([]string{}, recipients...)
	return nil
}

type fakeMessenger struct {
	sends    int
	messages []string
}

func (f *fakeMessenger) Send(_ context.Context, _ string, message string) error {
	f.sends++
	f.messages = append(f.messages, message)
	return nil
}
