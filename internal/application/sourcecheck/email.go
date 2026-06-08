package sourcecheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/sources"
	"github.com/codo/codo/internal/infra/store"
)

type EmailRouter interface {
	Run(ctx context.Context, t *task.Task) error
}

type EmailFetcher interface {
	FetchInbox(ctx context.Context, cfg sources.EmailInboxConfig) ([]sources.EmailMessage, error)
}

type EmailInboxFetcher struct{}

func (EmailInboxFetcher) FetchInbox(ctx context.Context, cfg sources.EmailInboxConfig) ([]sources.EmailMessage, error) {
	return sources.FetchEmailInbox(ctx, cfg)
}

type EmailService struct {
	store   Store
	router  EmailRouter
	fetcher EmailFetcher
	now     func() time.Time
}

func NewEmailService(st Store, router EmailRouter, fetcher EmailFetcher) *EmailService {
	if fetcher == nil {
		fetcher = EmailInboxFetcher{}
	}
	return &EmailService{store: st, router: router, fetcher: fetcher, now: time.Now}
}

type EmailRunResult struct {
	Items     int `json:"items"`
	Analyzed  int `json:"analyzed"`
	Important int `json:"important"`
	Normal    int `json:"normal"`
	Spam      int `json:"spam"`
	Failed    int `json:"failed"`
}

func (s *EmailService) Run(ctx context.Context, sub store.EmailSubscription) (EmailRunResult, error) {
	if s == nil || s.store == nil || s.router == nil || s.fetcher == nil {
		return EmailRunResult{}, fmt.Errorf("email source service not configured")
	}
	since := s.now().AddDate(0, 0, -sub.SinceDays)
	if sub.LastFetchedAt != nil {
		since = sub.LastFetchedAt.Add(-5 * time.Minute)
	}
	messages, err := s.fetcher.FetchInbox(ctx, sources.EmailInboxConfig{
		Account:     sub.Account,
		Password:    sub.Password,
		Host:        sub.Host,
		Port:        sub.Port,
		Mailbox:     sub.Mailbox,
		MaxMessages: sub.MaxMessages,
		Since:       since,
		UnreadOnly:  sub.SyncUnreadOnly,
	})
	if err != nil {
		_ = s.store.RecordSubscriptionFetchFailure(ctx, sub.ID, err)
		return EmailRunResult{}, err
	}

	var result EmailRunResult
	for _, msg := range messages {
		row, change, err := s.store.UpsertSourceItem(ctx, sourceInputFromEmail(sub, msg))
		if err != nil {
			return result, err
		}
		result.Items++
		if !change.Created && row.Status != "failed" && row.Status != "pending" {
			continue
		}
		if strings.TrimSpace(msg.Body) == "" {
			_ = s.store.UpdateSourceItemAnalysis(ctx, row.ID, "empty", "邮件正文为空，已跳过摘要。", "", []string{}, "")
			continue
		}
		t := task.New(row.ID, sub.UserID, task.SourceEmail, task.ContentEmail, msg.URL, buildEmailContent(sub, msg))
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = s.router.Run(runCtx, t)
		cancel()
		if err != nil {
			result.Failed++
			_ = s.store.UpdateSourceItemAnalysis(ctx, row.ID, "failed", err.Error(), "", []string{}, "")
			continue
		}
		status := emailStatusFromTask(t)
		switch status {
		case "important":
			result.Important++
		case "spam":
			result.Spam++
		default:
			result.Normal++
		}
		result.Analyzed++
		_ = s.store.UpdateSourceItemAnalysis(ctx, row.ID, status, t.Summary(), t.Category(), t.Tags(), t.ID)
	}
	if result.Failed > 0 {
		err := fmt.Errorf("email inbox: %d messages failed to analyze", result.Failed)
		_ = s.store.RecordSubscriptionFetchFailure(ctx, sub.ID, err)
		return result, err
	}
	if err := s.store.UpdateLastFetched(ctx, sub.ID); err != nil {
		return result, err
	}
	return result, nil
}

func sourceInputFromEmail(sub store.EmailSubscription, msg sources.EmailMessage) store.SourceItemInput {
	title := strings.TrimSpace(msg.Subject)
	if title == "" {
		title = "(无主题)"
	}
	payload := map[string]any{
		"account":     sub.Account,
		"mailbox":     sub.Mailbox,
		"from":        msg.From,
		"to":          msg.To,
		"subject":     title,
		"message_id":  msg.MessageID,
		"uid":         msg.UID,
		"received_at": msg.ReceivedAt.Format(time.RFC3339),
		"snippet":     msg.Snippet,
		"flags":       msg.Flags,
	}
	return store.SourceItemInput{
		UserID:         sub.UserID,
		SubscriptionID: sub.ID,
		SourceType:     "email",
		ItemType:       "email",
		ExternalID:     msg.ExternalID,
		Course:         msg.From,
		Title:          title,
		Status:         "pending",
		URL:            msg.URL,
		Payload:        payload,
	}
}

func buildEmailContent(sub store.EmailSubscription, msg sources.EmailMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "来源：个人邮箱\n")
	fmt.Fprintf(&b, "重要邮件单独提取：%t\n", sub.NotifyImportant)
	fmt.Fprintf(&b, "邮箱账号：%s\n", sub.Account)
	fmt.Fprintf(&b, "文件夹：%s\n", sub.Mailbox)
	if msg.Subject != "" {
		fmt.Fprintf(&b, "主题：%s\n", msg.Subject)
	}
	if msg.From != "" {
		fmt.Fprintf(&b, "发件人：%s\n", msg.From)
	}
	if msg.To != "" {
		fmt.Fprintf(&b, "收件人：%s\n", msg.To)
	}
	if !msg.ReceivedAt.IsZero() {
		fmt.Fprintf(&b, "时间：%s\n", msg.ReceivedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "\n正文：\n%s", msg.Body)
	return strings.TrimSpace(b.String())
}

func emailStatusFromTask(t *task.Task) string {
	switch t.FilterDecision() {
	case task.FilterPass:
		return "important"
	case task.FilterDiscard:
		return "spam"
	default:
		return "normal"
	}
}
