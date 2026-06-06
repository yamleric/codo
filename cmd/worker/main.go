package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/codo/codo/internal/application/ingest"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/codo/codo/internal/infra/notify"
	"github.com/codo/codo/internal/infra/store"
)

func main() {
	ctx := context.Background()

	// ── 基础设施初始化 ─────────────────────────────────────────────
	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := store.New(pool)

	llmClient := llm.NewClient(llm.Config{
		BaseURL: getenv("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:  getenv("LLM_API_KEY", ""),
		Model:   getenv("LLM_MODEL", "gpt-4o-mini"),
	})

	tg, err := notify.NewTelegram()
	if err != nil {
		slog.Warn("telegram not available", "err", err)
		tg = nil
	}

	var notifier pipeline.Notifier
	if tg != nil {
		notifier = tg
	} else {
		notifier = &logNotifier{}
	}

	// ── 组装 pipeline ─────────────────────────────────────────────
	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, notifier, onStatus),
		pipeline.NewVideo(fetcher.NewVideo(), llmClient, llmClient, st, notifier, onStatus),
	)
	if err != nil {
		slog.Error("router init", "err", err)
		os.Exit(1)
	}

	// ── 测试任务：抓取一个网页 ─────────────────────────────────────
	url := getenv("TEST_URL", "https://go.dev/blog/")
	normalizedURL, err := ingest.NormalizeURL(url)
	if err != nil {
		slog.Error("invalid test url", "err", err)
		os.Exit(1)
	}
	taskID := fmt.Sprintf("task-%d", time.Now().UnixMilli())
	t := task.New(taskID, "demo-user", task.SourceManual, ingest.DetectContentType(normalizedURL), normalizedURL, "")

	slog.Info("running task", "url", normalizedURL, "content_type", t.ContentType)
	if err := router.Run(ctx, t); err != nil {
		slog.Error("task failed", "err", err, "steps", len(t.Steps()))
	} else {
		slog.Info("task done",
			"status", t.Status(),
			"filter", t.FilterDecision(),
			"summary_len", len([]rune(t.Summary())),
		)
	}
}

func onStatus(snap task.Snapshot) {
	slog.Info("status", "task", snap.ID, "status", snap.Status)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// logNotifier prints to stdout when Telegram is not configured.
type logNotifier struct{}

func (l *logNotifier) Send(_ context.Context, userID, message string) error {
	slog.Info("notify", "user", userID, "msg_len", len(message),
		"preview", truncate(message, 100))
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
