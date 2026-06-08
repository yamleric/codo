package sourcecheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codo/codo/internal/infra/sources"
	"github.com/codo/codo/internal/infra/store"
)

type Store interface {
	UpsertSourceItem(ctx context.Context, input store.SourceItemInput) (store.SourceItemRow, store.SourceItemChange, error)
	UpdateLastFetched(ctx context.Context, subID string) error
	RecordSubscriptionFetchFailure(ctx context.Context, subID string, fetchErr error) error
	MarkSourceItemNewNotified(ctx context.Context, id string) error
	MarkSourceItemDueNotified(ctx context.Context, id string) error
	UpdateSourceItemAnalysis(ctx context.Context, id, status, summary, category string, tags []string, articleID string) error
}

type Notifier interface {
	Send(ctx context.Context, userID, message string) error
}

type ChaoxingFetcher interface {
	FetchItems(ctx context.Context, creds sources.ChaoxingCredentials) ([]sources.ChaoxingItem, error)
}

type ChaoxingService struct {
	store    Store
	notifier Notifier
	fetcher  ChaoxingFetcher
	now      func() time.Time
}

func NewChaoxingService(st Store, notifier Notifier, fetcher ChaoxingFetcher) *ChaoxingService {
	if fetcher == nil {
		fetcher = sources.NewChaoxingClient()
	}
	return &ChaoxingService{store: st, notifier: notifier, fetcher: fetcher, now: time.Now}
}

type ChaoxingRunResult struct {
	Items       int
	NewNotified int
	DueNotified int
}

func (s *ChaoxingService) Run(ctx context.Context, sub store.ChaoxingSubscription) (ChaoxingRunResult, error) {
	if s == nil || s.store == nil || s.fetcher == nil {
		return ChaoxingRunResult{}, fmt.Errorf("chaoxing source service not configured")
	}
	items, err := s.fetcher.FetchItems(ctx, sources.ChaoxingCredentials{
		Account:  sub.Account,
		Password: sub.Password,
		Cookie:   sub.Cookie,
	})
	if err != nil {
		_ = s.store.RecordSubscriptionFetchFailure(ctx, sub.ID, err)
		return ChaoxingRunResult{}, err
	}

	var result ChaoxingRunResult
	for _, item := range items {
		row, change, err := s.store.UpsertSourceItem(ctx, sourceInputFromChaoxing(sub, item))
		if err != nil {
			return result, err
		}
		result.Items++
		if sub.NotifyNew && change.Created && isActionableStatus(item.Status) {
			if err := s.sendAndMarkNew(ctx, sub, row); err == nil {
				result.NewNotified++
			}
		}
		if sub.NotifyDue && isDueSoon(row, s.now(), sub.AlertHours) && isActionableStatus(row.Status) && row.DueNotifiedAt == nil {
			if err := s.sendAndMarkDue(ctx, sub, row); err == nil {
				result.DueNotified++
			}
		}
	}
	if err := s.store.UpdateLastFetched(ctx, sub.ID); err != nil {
		return result, err
	}
	return result, nil
}

func (s *ChaoxingService) sendAndMarkNew(ctx context.Context, sub store.ChaoxingSubscription, item store.SourceItemRow) error {
	if s.notifier == nil {
		return nil
	}
	if err := s.notifier.Send(ctx, sub.UserID, buildChaoxingMessage("new", item, sub.AlertHours, s.now)); err != nil {
		return err
	}
	return s.store.MarkSourceItemNewNotified(ctx, item.ID)
}

func (s *ChaoxingService) sendAndMarkDue(ctx context.Context, sub store.ChaoxingSubscription, item store.SourceItemRow) error {
	if s.notifier == nil {
		return nil
	}
	if err := s.notifier.Send(ctx, sub.UserID, buildChaoxingMessage("due", item, sub.AlertHours, s.now)); err != nil {
		return err
	}
	return s.store.MarkSourceItemDueNotified(ctx, item.ID)
}

func sourceInputFromChaoxing(sub store.ChaoxingSubscription, item sources.ChaoxingItem) store.SourceItemInput {
	payload := map[string]any{
		"remaining_text": item.RemainingText,
		"raw_text":       item.RawText,
	}
	return store.SourceItemInput{
		UserID:         sub.UserID,
		SubscriptionID: sub.ID,
		SourceType:     "chaoxing",
		ItemType:       item.Type,
		ExternalID:     item.ExternalID,
		Course:         item.Course,
		Title:          item.Title,
		Status:         item.Status,
		URL:            item.URL,
		DueAt:          item.DueAt,
		Payload:        payload,
	}
}

func isActionableStatus(status string) bool {
	status = strings.TrimSpace(status)
	if status == "" {
		return true
	}
	if strings.Contains(status, "已完成") || strings.Contains(status, "已提交") || strings.Contains(status, "已过期") {
		return false
	}
	return strings.Contains(status, "未") || strings.Contains(status, "待") || strings.Contains(status, "进行")
}

func isDueSoon(item store.SourceItemRow, now time.Time, alertHours int) bool {
	if item.DueAt == nil {
		return false
	}
	if alertHours <= 0 {
		alertHours = 24
	}
	due := item.DueAt.In(now.Location())
	return due.After(now) && due.Sub(now) <= time.Duration(alertHours)*time.Hour
}

func buildChaoxingMessage(kind string, item store.SourceItemRow, alertHours int, now func() time.Time) string {
	title := "学习通新任务提醒"
	if kind == "due" {
		title = fmt.Sprintf("学习通临近截止提醒（%d小时内）", alertHours)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", title)
	fmt.Fprintf(&b, "类型：%s\n", readableItemType(item.ItemType))
	if item.Course != "" {
		fmt.Fprintf(&b, "课程：%s\n", item.Course)
	}
	fmt.Fprintf(&b, "名称：%s\n", fallback(item.Title, "未命名任务"))
	if item.Status != "" {
		fmt.Fprintf(&b, "状态：%s\n", item.Status)
	}
	if item.DueAt != nil {
		fmt.Fprintf(&b, "截止：%s", item.DueAt.Format("2006-01-02 15:04"))
		if now != nil {
			remain := item.DueAt.Sub(now())
			if remain > 0 {
				fmt.Fprintf(&b, "（剩余%s）", readableDuration(remain))
			}
		}
		fmt.Fprintln(&b)
	}
	if item.URL != "" {
		fmt.Fprintf(&b, "链接：%s\n", item.URL)
	}
	return strings.TrimSpace(b.String())
}

func readableItemType(value string) string {
	switch value {
	case "exam":
		return "考试"
	default:
		return "作业"
	}
}

func readableDuration(d time.Duration) string {
	if d < 0 {
		return "已截止"
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours >= 24 {
		return fmt.Sprintf("%d天%d小时", hours/24, hours%24)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

func fallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
