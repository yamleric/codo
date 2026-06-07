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
	knowledgeapp "github.com/codo/codo/internal/application/knowledge"
	"github.com/codo/codo/internal/application/pipeline"
	dailyreport "github.com/codo/codo/internal/application/report"
	"github.com/codo/codo/internal/application/sourcecheck"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/runtimeconfig"
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
	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("db migrate", "err", err)
		os.Exit(1)
	}

	st := store.New(pool)
	llmClient := &runtimeconfig.LLM{Store: st}
	embeddingClient := &runtimeconfig.Embedder{Store: st}

	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, &runtimeconfig.Notifier{Store: st}, nil),
		pipeline.NewVideo(fetcher.NewVideo(), llmClient, llmClient, st, &runtimeconfig.Notifier{Store: st}, nil),
	)
	if err != nil {
		slog.Error("router init", "err", err)
		os.Exit(1)
	}
	reportService := dailyReportService(st)
	knowledgeService := knowledgeapp.NewService(st, nil, embeddingClient)

	interval := 30 * time.Minute
	slog.Info("scheduler started", "interval", interval)
	tick := time.NewTicker(interval)
	defer tick.Stop()

	// run immediately on start
	runRSS(ctx, st, router)
	runChaoxing(ctx, st)
	runDailyReport(ctx, reportService)
	runEmbeddingBackfill(ctx, knowledgeService)

	for {
		select {
		case <-tick.C:
			runRSS(ctx, st, router)
			runChaoxing(ctx, st)
			runDailyReport(ctx, reportService)
			runEmbeddingBackfill(ctx, knowledgeService)
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		}
	}
}

func runRSS(ctx context.Context, st *store.Store, router *pipeline.Router) {
	subs, err := st.ListActiveRSSSubscriptions(ctx)
	if err != nil {
		slog.Error("list rss subs", "err", err)
		return
	}
	slog.Info("rss run", "subscriptions", len(subs))

	for _, sub := range subs {
		var since time.Time
		if sub.LastFetchedAt != nil {
			since = *sub.LastFetchedAt
		}

		items, err := sources.FetchRSS(ctx, sub.FeedURL, since, 20)
		if err != nil {
			_ = st.RecordRSSFetchFailure(ctx, sub.ID, err)
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

func runChaoxing(ctx context.Context, st *store.Store) {
	subs, err := st.ListActiveChaoxingSubscriptions(ctx)
	if err != nil {
		slog.Error("list chaoxing subs", "err", err)
		return
	}
	if len(subs) == 0 {
		return
	}
	slog.Info("chaoxing run", "subscriptions", len(subs))
	service := sourcecheck.NewChaoxingService(st, &runtimeconfig.Notifier{Store: st}, nil)
	for _, sub := range subs {
		runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		result, err := service.Run(runCtx, sub)
		cancel()
		if err != nil {
			slog.Warn("chaoxing fetch failed", "subscription", sub.ID, "err", err)
			continue
		}
		slog.Info("chaoxing fetched", "subscription", sub.ID, "items", result.Items, "new_notified", result.NewNotified, "due_notified", result.DueNotified)
	}
}

func dailyReportService(st *store.Store) *dailyreport.Service {
	return dailyreport.NewService(st, &runtimeconfig.EmailSender{Store: st})
}

func runDailyReport(ctx context.Context, service *dailyreport.Service) {
	if service == nil {
		return
	}
	userID := getenv("DEFAULT_USER_ID", "demo-user")
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := service.RunForUser(runCtx, userID, time.Now())
	if err != nil {
		slog.Warn("daily report failed", "status", result.Status, "date", result.ReportDate, "items", result.ItemCount, "err", err)
		return
	}
	switch result.Status {
	case "sent", "skipped":
		slog.Info("daily report completed", "status", result.Status, "date", result.ReportDate, "items", result.ItemCount)
	case "disabled", "pending", "already_done":
	default:
		slog.Info("daily report checked", "status", result.Status, "date", result.ReportDate, "items", result.ItemCount)
	}
}

func runEmbeddingBackfill(ctx context.Context, service *knowledgeapp.Service) {
	if service == nil {
		return
	}
	userID := getenv("DEFAULT_USER_ID", "demo-user")
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	result, err := service.BackfillEmbeddings(runCtx, userID, 24)
	if err != nil {
		slog.Warn("knowledge embedding backfill failed", "scanned", result.Scanned, "embedded", result.Embedded, "failed", result.Failed, "err", err)
		return
	}
	if result.Embedded > 0 || result.Failed > 0 {
		slog.Info("knowledge embedding backfill completed", "scanned", result.Scanned, "embedded", result.Embedded, "failed", result.Failed)
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
