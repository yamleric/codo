package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/codo/codo/internal/infra/store"
)

type Store interface {
	GetUserSettings(ctx context.Context, userID string) (store.UserSettings, error)
	ListDailyArticles(ctx context.Context, userID string, start, end time.Time, limit int) ([]store.DailyArticleRow, error)
	StartDailyReport(ctx context.Context, userID, reportDate string) (bool, error)
	MarkDailyReportSent(ctx context.Context, userID, reportDate string, itemCount int) error
	MarkDailyReportSkipped(ctx context.Context, userID, reportDate string, itemCount int) error
	MarkDailyReportFailed(ctx context.Context, userID, reportDate string, reportErr error) error
}

type EmailSender interface {
	Send(ctx context.Context, recipients []string, subject, body string) error
}

type Service struct {
	store Store
	email EmailSender
}

func NewService(st Store, email EmailSender) *Service {
	return &Service{store: st, email: email}
}

type RunResult struct {
	ReportDate string
	ItemCount  int
	Status     string
}

func (s *Service) RunForUser(ctx context.Context, userID string, now time.Time) (RunResult, error) {
	if s == nil || s.store == nil {
		return RunResult{Status: "disabled"}, fmt.Errorf("daily report: store not configured")
	}
	settings, err := s.store.GetUserSettings(ctx, userID)
	if err != nil {
		return RunResult{}, err
	}
	cfg := store.NormalizeDailyReport(settings.DailyReport)
	if !cfg.Enabled || cfg.Email == "" {
		return RunResult{Status: "disabled"}, nil
	}
	if s.email == nil {
		return RunResult{Status: "disabled"}, fmt.Errorf("daily report: email sender not configured")
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	localNow := now.In(loc)
	if localNow.Hour() < cfg.Hour {
		return RunResult{ReportDate: localNow.Format("2006-01-02"), Status: "pending"}, nil
	}

	reportDate := localNow.Format("2006-01-02")
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	started, err := s.store.StartDailyReport(ctx, userID, reportDate)
	if err != nil {
		return RunResult{ReportDate: reportDate}, err
	}
	if !started {
		return RunResult{ReportDate: reportDate, Status: "already_done"}, nil
	}

	articles, err := s.store.ListDailyArticles(ctx, userID, start, end, cfg.MaxItems)
	if err != nil {
		_ = s.store.MarkDailyReportFailed(ctx, userID, reportDate, err)
		return RunResult{ReportDate: reportDate, Status: "failed"}, err
	}
	if len(articles) == 0 {
		if err := s.store.MarkDailyReportSkipped(ctx, userID, reportDate, 0); err != nil {
			return RunResult{ReportDate: reportDate, Status: "failed"}, err
		}
		return RunResult{ReportDate: reportDate, Status: "skipped"}, nil
	}

	subject := fmt.Sprintf("Codo 日报 %s（%d 条）", reportDate, len(articles))
	body := BuildDailyEmailBody(reportDate, cfg.Timezone, articles)
	if err := s.email.Send(ctx, []string{cfg.Email}, subject, body); err != nil {
		_ = s.store.MarkDailyReportFailed(ctx, userID, reportDate, err)
		return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "failed"}, err
	}
	if err := s.store.MarkDailyReportSent(ctx, userID, reportDate, len(articles)); err != nil {
		return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "failed"}, err
	}
	return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "sent"}, nil
}

func BuildDailyEmailBody(reportDate, timezone string, articles []store.DailyArticleRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Codo 日报\n")
	fmt.Fprintf(&b, "日期：%s\n", reportDate)
	fmt.Fprintf(&b, "时区：%s\n", timezone)
	fmt.Fprintf(&b, "条目：%d\n\n", len(articles))

	grouped := groupByCategory(articles)
	categories := make([]string, 0, len(grouped))
	for category := range grouped {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	for _, category := range categories {
		fmt.Fprintf(&b, "## %s\n\n", category)
		for idx, article := range grouped[category] {
			fmt.Fprintf(&b, "%d. %s\n", idx+1, firstLine(article.Summary))
			fmt.Fprintf(&b, "   来源：%s", readableSource(article.Source))
			if len(article.Tags) > 0 {
				fmt.Fprintf(&b, " | 标签：%s", strings.Join(article.Tags, " / "))
			}
			fmt.Fprintf(&b, "\n")
			if rest := remainingLines(article.Summary); rest != "" {
				fmt.Fprintf(&b, "   %s\n", indentContinuation(rest))
			}
			if strings.TrimSpace(article.URL) != "" {
				fmt.Fprintf(&b, "   链接：%s\n", article.URL)
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func groupByCategory(articles []store.DailyArticleRow) map[string][]store.DailyArticleRow {
	grouped := make(map[string][]store.DailyArticleRow)
	for _, article := range articles {
		category := strings.TrimSpace(article.Category)
		if category == "" {
			category = "未分类"
		}
		grouped[category] = append(grouped[category], article)
	}
	return grouped
}

func firstLine(text string) string {
	lines := summaryLines(text)
	if len(lines) == 0 {
		return "无摘要"
	}
	return lines[0]
}

func remainingLines(text string) string {
	lines := summaryLines(text)
	if len(lines) <= 1 {
		return ""
	}
	return strings.Join(lines[1:], "\n")
}

func summaryLines(text string) []string {
	rawLines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func indentContinuation(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n   ")
}

func readableSource(source string) string {
	switch source {
	case "rss":
		return "RSS"
	case "manual":
		return "手动收藏"
	case "bookmark":
		return "收藏夹"
	case "":
		return "未知"
	default:
		return source
	}
}
