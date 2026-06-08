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
	ListDailyArticles(ctx context.Context, userID string, start, end time.Time, query store.DailyArticleQuery) ([]store.DailyArticleRow, error)
	StartDailyReport(ctx context.Context, userID, reportDate string) (bool, error)
	MarkDailyReportSent(ctx context.Context, userID, reportDate string, itemCount int) error
	MarkDailyReportSkipped(ctx context.Context, userID, reportDate string, itemCount int) error
	MarkDailyReportFailed(ctx context.Context, userID, reportDate string, reportErr error) error
}

type EmailSender interface {
	Send(ctx context.Context, recipients []string, subject, body string) error
}

type MessageSender interface {
	Send(ctx context.Context, userID, message string) error
}

type Service struct {
	store     Store
	email     EmailSender
	messenger MessageSender
}

func NewService(st Store, email EmailSender, messengers ...MessageSender) *Service {
	var messenger MessageSender
	if len(messengers) > 0 {
		messenger = messengers[0]
	}
	return &Service{store: st, email: email, messenger: messenger}
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
	recipient := store.DailyReportRecipient(cfg, settings.Username)
	if !cfg.Enabled || recipient == "" {
		if cfg.Enabled && !store.ReportUsesChannel(cfg, "email") {
			recipient = ""
		} else {
			return RunResult{Status: "disabled"}, nil
		}
	}
	if !cfg.Enabled {
		return RunResult{Status: "disabled"}, nil
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	localNow := now.In(loc)
	period, ok := reportPeriod(cfg, localNow, loc)
	if !ok {
		return RunResult{ReportDate: pendingReportDate(cfg, localNow, loc), Status: "pending"}, nil
	}

	reportDate := period.Start.Format("2006-01-02")

	started, err := s.store.StartDailyReport(ctx, userID, reportDate)
	if err != nil {
		return RunResult{ReportDate: reportDate}, err
	}
	if !started {
		return RunResult{ReportDate: reportDate, Status: "already_done"}, nil
	}

	articles, err := s.store.ListDailyArticles(ctx, userID, period.Start, period.End, store.DailyArticleQuery{
		Limit:        cfg.MaxItems,
		Sources:      cfg.Sources,
		Categories:   cfg.Categories,
		CategoryMode: cfg.CategoryMode,
	})
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

	if err := s.sendReport(ctx, userID, recipient, cfg, period, articles); err != nil {
		_ = s.store.MarkDailyReportFailed(ctx, userID, reportDate, err)
		return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "failed"}, err
	}
	if err := s.store.MarkDailyReportSent(ctx, userID, reportDate, len(articles)); err != nil {
		return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "failed"}, err
	}
	return RunResult{ReportDate: reportDate, ItemCount: len(articles), Status: "sent"}, nil
}

type reportPeriodRange struct {
	Label string
	Start time.Time
	End   time.Time
}

func reportPeriod(cfg store.DailyReport, localNow time.Time, loc *time.Location) (reportPeriodRange, bool) {
	if localNow.Hour() < cfg.Hour {
		return reportPeriodRange{}, false
	}
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	switch cfg.Frequency {
	case "weekly":
		if localNow.Weekday() != time.Monday {
			return reportPeriodRange{}, false
		}
		start := today.AddDate(0, 0, -7)
		return reportPeriodRange{Label: "周报", Start: start, End: today}, true
	case "monthly":
		if localNow.Day() != 1 {
			return reportPeriodRange{}, false
		}
		start := today.AddDate(0, -1, 0)
		return reportPeriodRange{Label: "月报", Start: start, End: today}, true
	default:
		return reportPeriodRange{Label: "日报", Start: today, End: today.Add(24 * time.Hour)}, true
	}
}

func pendingReportDate(cfg store.DailyReport, localNow time.Time, loc *time.Location) string {
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	switch cfg.Frequency {
	case "weekly":
		daysSinceMonday := (int(localNow.Weekday()) + 6) % 7
		return today.AddDate(0, 0, -daysSinceMonday-7).Format("2006-01-02")
	case "monthly":
		return today.AddDate(0, -1, -localNow.Day()+1).Format("2006-01-02")
	default:
		return today.Format("2006-01-02")
	}
}

func (s *Service) sendReport(ctx context.Context, userID, recipient string, cfg store.DailyReport, period reportPeriodRange, articles []store.DailyArticleRow) error {
	if cfg.SplitByCategory {
		grouped := groupByCategory(articles)
		categories := sortedCategories(grouped)
		for _, category := range categories {
			items := grouped[category]
			subject := reportSubject(period, category, len(items))
			body := BuildReportBody(period, cfg.Timezone, items)
			if err := s.sendToChannels(ctx, userID, recipient, cfg, subject, body); err != nil {
				return err
			}
		}
		return nil
	}
	subject := reportSubject(period, "", len(articles))
	body := BuildReportBody(period, cfg.Timezone, articles)
	return s.sendToChannels(ctx, userID, recipient, cfg, subject, body)
}

func (s *Service) sendToChannels(ctx context.Context, userID, recipient string, cfg store.DailyReport, subject, body string) error {
	for _, channel := range cfg.Channels {
		switch channel {
		case "email":
			if s.email == nil {
				return fmt.Errorf("daily report: email sender not configured")
			}
			if recipient == "" {
				return fmt.Errorf("daily report: email recipient not set")
			}
			if err := s.email.Send(ctx, []string{recipient}, subject, body); err != nil {
				return err
			}
		case "telegram":
			if s.messenger == nil {
				return fmt.Errorf("daily report: telegram sender not configured")
			}
			for _, chunk := range messageChunks(subject+"\n\n"+body, 3600) {
				if err := s.messenger.Send(ctx, userID, chunk); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func reportSubject(period reportPeriodRange, category string, count int) string {
	label := period.Label
	if label == "" {
		label = "日报"
	}
	if !period.End.IsZero() && period.End.Sub(period.Start) > 25*time.Hour {
		label = fmt.Sprintf("Codo %s %s 至 %s", label, period.Start.Format("2006-01-02"), period.End.AddDate(0, 0, -1).Format("2006-01-02"))
	} else {
		label = fmt.Sprintf("Codo %s %s", label, period.Start.Format("2006-01-02"))
	}
	if strings.TrimSpace(category) != "" {
		label += " - " + strings.TrimSpace(category)
	}
	return fmt.Sprintf("%s（%d 条）", label, count)
}

func BuildDailyEmailBody(reportDate, timezone string, articles []store.DailyArticleRow) string {
	period := reportPeriodRange{Label: "日报"}
	if start, err := time.Parse("2006-01-02", reportDate); err == nil {
		period.Start = start
		period.End = start.Add(24 * time.Hour)
	}
	return BuildReportBody(period, timezone, articles)
}

func BuildReportBody(period reportPeriodRange, timezone string, articles []store.DailyArticleRow) string {
	var b strings.Builder
	label := period.Label
	if label == "" {
		label = "日报"
	}
	fmt.Fprintf(&b, "Codo %s\n", label)
	if !period.Start.IsZero() && !period.End.IsZero() && period.End.Sub(period.Start) > 25*time.Hour {
		fmt.Fprintf(&b, "周期：%s 至 %s\n", period.Start.Format("2006-01-02"), period.End.AddDate(0, 0, -1).Format("2006-01-02"))
	} else if !period.Start.IsZero() {
		fmt.Fprintf(&b, "日期：%s\n", period.Start.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "时区：%s\n", timezone)
	fmt.Fprintf(&b, "条目：%d\n\n", len(articles))

	grouped := groupByCategory(articles)
	for _, category := range sortedCategories(grouped) {
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

func sortedCategories(grouped map[string][]store.DailyArticleRow) []string {
	categories := make([]string, 0, len(grouped))
	for category := range grouped {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
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

func messageChunks(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{}
	}
	if maxRunes <= 0 {
		maxRunes = 3600
	}
	var chunks []string
	var current strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if current.Len() > 0 && len([]rune(current.String()))+len([]rune(line))+1 > maxRunes {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
		if len([]rune(line)) > maxRunes {
			runes := []rune(line)
			for len(runes) > 0 {
				n := maxRunes
				if len(runes) < n {
					n = len(runes)
				}
				if current.Len() > 0 {
					chunks = append(chunks, strings.TrimSpace(current.String()))
					current.Reset()
				}
				chunks = append(chunks, string(runes[:n]))
				runes = runes[n:]
			}
			continue
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}
	if strings.TrimSpace(current.String()) != "" {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	return chunks
}

func readableSource(source string) string {
	switch source {
	case "rss":
		return "RSS"
	case "manual":
		return "手动收藏"
	case "bookmark":
		return "收藏夹"
	case "linux_do":
		return "linux.do"
	case "wechat_mp":
		return "公众号"
	case "email":
		return "邮件"
	case "chaoxing":
		return "学习通"
	case "group_chat":
		return "群消息"
	case "":
		return "未知"
	default:
		return source
	}
}
