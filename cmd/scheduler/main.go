package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codo/codo/internal/application/ingest"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/codo/codo/internal/infra/sources"
	"github.com/codo/codo/internal/infra/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}

	st := store.New(pool)
	llmClient := llm.NewClient(llm.Config{
		BaseURL: getenv("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:  getenv("LLM_API_KEY", ""),
		Model:   getenv("LLM_MODEL", "gpt-4o-mini"),
	})

	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, &logNotifier{}, nil),
		pipeline.NewVideo(fetcher.NewVideo(), llmClient, llmClient, st, &logNotifier{}, nil),
	)
	if err != nil {
		slog.Error("router init", "err", err)
		os.Exit(1)
	}

	interval := 30 * time.Minute
	slog.Info("scheduler started", "interval", interval)
	tick := time.NewTicker(interval)
	defer tick.Stop()

	// run immediately on start
	runRSS(ctx, st, router)

	for {
		select {
		case <-tick.C:
			runRSS(ctx, st, router)
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		}
	}
}

func runRSS(ctx context.Context, st *store.Store, router *pipeline.Router) {
	subs, err := st.ListRSSSubscriptions(ctx)
	if err != nil {
		slog.Error("list rss subs", "err", err)
		return
	}
	slog.Info("rss run", "subscriptions", len(subs))

	for _, sub := range subs {
		var since time.Time
		if sub.LastFetchedAt != nil {
			if t, ok := sub.LastFetchedAt.(time.Time); ok {
				since = t
			}
		}

		items, err := sources.FetchRSS(ctx, sub.FeedURL, since, 20)
		if err != nil {
			slog.Warn("rss fetch failed", "url", sub.FeedURL, "err", err)
			continue
		}
		slog.Info("rss fetched", "url", sub.FeedURL, "items", len(items))

		for _, item := range items {
			contentType := task.ContentWebPage
			if normalizedURL, err := ingest.NormalizeURL(item.URL); err == nil {
				item.URL = normalizedURL
				contentType = ingest.DetectContentType(normalizedURL)
			}
			t := task.New(
				fmt.Sprintf("rss-%d", time.Now().UnixNano()),
				sub.UserID,
				task.SourceRSS,
				contentType,
				item.URL,
				"",
			)
			runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			if err := router.Run(runCtx, t); err != nil {
				slog.Warn("rss task failed", "url", item.URL, "err", err)
			}
			cancel()
		}

		_ = st.UpdateLastFetched(ctx, sub.ID)
	}
}

type logNotifier struct{}

func (l *logNotifier) Send(_ context.Context, userID, message string) error {
	slog.Info("notify", "user", userID, "len", len(message))
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
