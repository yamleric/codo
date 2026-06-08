package runtimeconfig

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/codo/codo/internal/infra/notify"
	"github.com/codo/codo/internal/infra/store"
)

type LLM struct {
	Store *store.Store
}

func (r *LLM) client(ctx context.Context) *llm.Client {
	cfg := Resolved(ctx, r.Store)
	return llm.NewClient(llm.Config{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.Model,
		Preferences: r.Store,
	})
}

func (r *LLM) Configured() bool {
	return r.client(context.Background()).Configured()
}

func (r *LLM) Complete(ctx context.Context, system, user string) (string, error) {
	return r.client(ctx).Complete(ctx, system, user)
}

func (r *LLM) Filter(ctx context.Context, userID, content string) (task.FilterDecision, string, error) {
	return r.client(ctx).Filter(ctx, userID, content)
}

func (r *LLM) Summarize(ctx context.Context, t *task.Task, content string) (string, error) {
	return r.client(ctx).Summarize(ctx, t, content)
}

func (r *LLM) Classify(ctx context.Context, content string) (string, error) {
	return r.client(ctx).Classify(ctx, content)
}

func (r *LLM) Categorize(ctx context.Context, userID, content string) (task.Classification, error) {
	return r.client(ctx).Categorize(ctx, userID, content)
}

type Embedder struct {
	Store *store.Store
}

func (r *Embedder) client(ctx context.Context) *llm.EmbeddingClient {
	cfg := Resolved(ctx, r.Store)
	return llm.NewEmbeddingClient(llm.EmbeddingConfig{
		BaseURL: cfg.Embedding.BaseURL,
		APIKey:  cfg.Embedding.APIKey,
		Model:   cfg.Embedding.Model,
	})
}

func (r *Embedder) Configured() bool {
	return r.client(context.Background()).Configured()
}

func (r *Embedder) Embed(ctx context.Context, input string) ([]float32, error) {
	return r.client(ctx).Embed(ctx, input)
}

type Notifier struct {
	Store *store.Store
}

func (n *Notifier) Send(ctx context.Context, userID, message string) error {
	settings, err := n.Store.GetUserSettings(ctx, userID)
	if err != nil {
		return err
	}
	cfg := Resolved(ctx, n.Store)
	switch settings.NotifyChannel {
	case "none":
		return nil
	case "email":
		recipient := store.DailyReportRecipient(settings.DailyReport, settings.Username)
		if recipient == "" {
			return fmt.Errorf("notify: email recipient not set")
		}
		email, err := notify.NewEmail(EmailConfig(cfg.SMTP))
		if err != nil {
			return err
		}
		return email.Send(ctx, []string{recipient}, "Codo 内容推送", message)
	default:
		if strings.TrimSpace(cfg.Telegram.ChatID) == "" {
			return fmt.Errorf("notify: telegram chat id not set")
		}
		telegram, err := notify.NewTelegramWithToken(cfg.Telegram.Token)
		if err != nil {
			return err
		}
		return telegram.Send(ctx, cfg.Telegram.ChatID, message)
	}
}

type EmailSender struct {
	Store *store.Store
}

func (s *EmailSender) Send(ctx context.Context, recipients []string, subject, body string) error {
	cfg := Resolved(ctx, s.Store)
	email, err := notify.NewEmail(EmailConfig(cfg.SMTP))
	if err != nil {
		return err
	}
	return email.Send(ctx, recipients, subject, body)
}

type TelegramSender struct {
	Store *store.Store
}

func (s *TelegramSender) Send(ctx context.Context, userID, message string) error {
	cfg := Resolved(ctx, s.Store)
	if strings.TrimSpace(cfg.Telegram.ChatID) == "" {
		return fmt.Errorf("notify: telegram chat id not set")
	}
	telegram, err := notify.NewTelegramWithToken(cfg.Telegram.Token)
	if err != nil {
		return err
	}
	return telegram.Send(ctx, cfg.Telegram.ChatID, message)
}

func EmailConfig(config store.SMTPRuntimeConfig) notify.EmailConfig {
	return notify.EmailConfig{
		Host:     config.Host,
		Port:     config.Port,
		Username: config.Username,
		Password: config.Password,
		From:     config.From,
		UseTLS:   config.UseTLS,
		Timeout:  20 * time.Second,
	}
}

func Resolved(ctx context.Context, st *store.Store) store.AppConfig {
	config, err := st.GetAppConfig(ctx)
	if err != nil {
		slog.Warn("load app config failed", "err", err)
		config = store.DefaultAppConfig()
	}
	return store.MergeAppConfig(config, FromEnv())
}

func FromEnv() store.AppConfig {
	llmBaseURL := getenv("LLM_BASE_URL", "https://api.openai.com/v1")
	return store.NormalizeAppConfig(store.AppConfig{
		LLM: store.LLMRuntimeConfig{
			BaseURL: llmBaseURL,
			APIKey:  os.Getenv("LLM_API_KEY"),
			Model:   getenv("LLM_MODEL", "gpt-4o-mini"),
		},
		Embedding: store.LLMRuntimeConfig{
			BaseURL: getenv("EMBEDDING_BASE_URL", llmBaseURL),
			APIKey:  os.Getenv("EMBEDDING_API_KEY"),
			Model:   getenv("EMBEDDING_MODEL", "text-embedding-3-small"),
		},
		ASR: store.ASRRuntimeConfig{
			BaseURL: os.Getenv("ASR_BASE_URL"),
			APIKey:  os.Getenv("ASR_API_KEY"),
			Model:   getenv("ASR_MODEL", "whisper-1"),
		},
		Telegram: store.TelegramRuntimeConfig{
			Token:  os.Getenv("TELEGRAM_TOKEN"),
			ChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		},
		SMTP: store.SMTPRuntimeConfig{
			Host:     os.Getenv("SMTP_HOST"),
			Port:     getenvInt("SMTP_PORT", 587),
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
			From:     os.Getenv("SMTP_FROM"),
			UseTLS:   getenvBool("SMTP_USE_TLS", getenvInt("SMTP_PORT", 587) == 465),
		},
	})
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getenvBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
