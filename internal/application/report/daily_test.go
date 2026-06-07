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

type fakeStore struct {
	settings store.UserSettings
	articles []store.DailyArticleRow
	started  bool
	sent     int
}

func (f *fakeStore) GetUserSettings(context.Context, string) (store.UserSettings, error) {
	return f.settings, nil
}

func (f *fakeStore) ListDailyArticles(context.Context, string, time.Time, time.Time, int) ([]store.DailyArticleRow, error) {
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
	sends int
}

func (f *fakeEmail) Send(context.Context, []string, string, string) error {
	f.sends++
	return nil
}
