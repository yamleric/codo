package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codo/codo/internal/application/ingest"
	knowledgeapp "github.com/codo/codo/internal/application/knowledge"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/application/sourcecheck"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/runtimeconfig"
	"github.com/codo/codo/internal/infra/sources"
	"github.com/codo/codo/internal/infra/store"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

// ── WebSocket hub ─────────────────────────────────────────────────────────────

type hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func (h *hub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *hub) broadcast(snap task.Snapshot) {
	b, _ := json.Marshal(snap)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		_ = c.WriteMessage(websocket.TextMessage, b)
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

// ── Server ────────────────────────────────────────────────────────────────────

type server struct {
	st        *store.Store
	router    *pipeline.Router
	hub       *hub
	knowledge *knowledgeapp.Service
}

func (s *server) onStatus(snap task.Snapshot) {
	s.hub.broadcast(snap)
}

// POST /api/tasks
func (s *server) createTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	normalizedURL, err := ingest.NormalizeURL(body.URL)
	if err != nil {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	contentType := ingest.DetectContentType(normalizedURL)

	userID := defaultUserID()
	id := fmt.Sprintf("task-%d", time.Now().UnixMilli())
	t := task.New(id, userID, task.SourceManual, contentType, normalizedURL, "")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout(contentType))
		defer cancel()
		if err := s.router.Run(ctx, t); err != nil {
			slog.Error("task failed", "id", id, "err", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// GET /api/tasks
func (s *server) listTasks(w http.ResponseWriter, r *http.Request) {
	userID := defaultUserID()
	tasks, err := s.st.ListTasks(r.Context(), userID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// POST /api/tasks/:id/retry
func (s *server) retryTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	id = strings.TrimSuffix(id, "/retry")

	existing, err := s.st.GetTask(r.Context(), id)
	if err != nil || existing == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	userID := defaultUserID()
	newID := fmt.Sprintf("task-%d", time.Now().UnixMilli())
	t := task.New(newID, userID, task.SourceType(existing.Source),
		task.ContentType(existing.ContentType), existing.URL, "")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout(t.ContentType))
		defer cancel()
		_ = s.router.Run(ctx, t)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": newID})
}

// GET /api/settings
// PATCH /api/settings
func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,PATCH,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	userID := defaultUserID()
	current, err := s.st.GetUserSettings(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := runtimeconfig.Resolved(r.Context(), s.st)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponseFromStore(current, config))
	case http.MethodPatch:
		var body settingsPatch
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		updated, err := applySettingsPatch(current, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.st.UpdateUserSettings(r.Context(), updated); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		appConfig, err := s.st.GetAppConfig(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		appConfig, err = applyRuntimeConfigPatch(appConfig, body.RuntimeConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.RuntimeConfig != nil {
			if err := s.st.SaveAppConfig(r.Context(), appConfig); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		resolvedConfig := store.MergeAppConfig(appConfig, runtimeconfig.FromEnv())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponseFromStore(updated, resolvedConfig))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /ws
func (s *server) wsHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.add(c)
	defer func() {
		s.hub.remove(c)
		c.Close()
	}()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
}

// GET /api/subscriptions — list subscriptions
// POST /api/subscriptions — add RSS or Chaoxing subscription
func (s *server) subscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	userID := defaultUserID()
	switch r.Method {
	case http.MethodGet:
		subs, err := s.st.ListSubscriptions(r.Context(), userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)
	case http.MethodPost:
		var body subscriptionPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		sourceType := subscriptionSourceType(body)
		var id string
		var err error
		switch sourceType {
		case "rss":
			if body.FeedURL == nil {
				http.Error(w, "invalid feed_url", http.StatusBadRequest)
				return
			}
			feedURL, normalizeErr := normalizeFeedURL(*body.FeedURL)
			if normalizeErr != nil {
				http.Error(w, "invalid feed_url", http.StatusBadRequest)
				return
			}
			id, err = s.st.AddRSSSubscription(r.Context(), userID, feedURL, stringValue(body.Title), stringValue(body.Category))
		case "chaoxing":
			id, err = s.st.AddChaoxingSubscription(r.Context(), userID, chaoxingInputFromPayload(body, nil))
		default:
			http.Error(w, "invalid source_type", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), statusFromStoreError(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// PATCH /api/subscriptions/:id — update subscription
// DELETE /api/subscriptions/:id — delete subscription
// POST /api/subscriptions/:id/refresh — fetch subscription items now
func (s *server) subscriptionByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "PATCH,DELETE,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/subscriptions/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	id := parts[0]
	userID := defaultUserID()

	if len(parts) == 2 && parts[1] == "refresh" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.refreshSubscription(w, r, userID, id)
		return
	}
	if len(parts) != 1 {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		existing, err := s.st.GetSourceSubscription(r.Context(), userID, id)
		if err != nil {
			http.Error(w, "subscription not found", http.StatusNotFound)
			return
		}
		var body subscriptionPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		switch existing.SourceType {
		case "rss":
			feedURL := existing.FeedURL
			if body.FeedURL != nil {
				normalized, err := normalizeFeedURL(*body.FeedURL)
				if err != nil {
					http.Error(w, "invalid feed_url", http.StatusBadRequest)
					return
				}
				feedURL = normalized
			}
			title := existing.Title
			if body.Title != nil {
				title = *body.Title
			}
			category := existing.Category
			if body.Category != nil {
				category = *body.Category
			}
			enabled := existing.Enabled
			if body.Enabled != nil {
				enabled = *body.Enabled
			}
			if err := s.st.UpdateRSSSubscription(r.Context(), userID, id, feedURL, title, category, enabled); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "chaoxing":
			if err := s.st.UpdateChaoxingSubscription(r.Context(), userID, id, chaoxingInputFromPayload(body, existing)); err != nil {
				http.Error(w, err.Error(), statusFromStoreError(err))
				return
			}
		default:
			http.Error(w, "unsupported subscription type", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.st.DeleteSubscription(r.Context(), userID, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) refreshSubscription(w http.ResponseWriter, r *http.Request, userID, id string) {
	generic, err := s.st.GetSourceSubscription(r.Context(), userID, id)
	if err != nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	if generic.SourceType == "chaoxing" {
		sub, err := s.st.GetChaoxingSubscription(r.Context(), userID, id)
		if err != nil {
			http.Error(w, "subscription not found", http.StatusNotFound)
			return
		}
		runCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		result, err := sourcecheck.NewChaoxingService(s.st, &runtimeconfig.Notifier{Store: s.st}, nil).Run(runCtx, *sub)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	sub, err := s.st.GetRSSSubscription(r.Context(), userID, id)
	if err != nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}
	var since time.Time
	if sub.LastFetchedAt != nil {
		since = *sub.LastFetchedAt
	}
	items, err := sources.FetchRSS(r.Context(), sub.FeedURL, since, 20)
	if err != nil {
		_ = s.st.RecordRSSFetchFailure(r.Context(), sub.ID, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for _, item := range items {
		s.runRSSItem(sub.UserID, item)
	}
	_ = s.st.UpdateLastFetched(r.Context(), sub.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"items": len(items)})
}

func (s *server) runRSSItem(userID string, item sources.RSSItem) {
	contentType := task.ContentWebPage
	if normalizedURL, err := ingest.NormalizeURL(item.URL); err == nil {
		item.URL = normalizedURL
		contentType = ingest.DetectContentType(normalizedURL)
	}
	t := task.New(
		fmt.Sprintf("rss-%d", time.Now().UnixNano()),
		userID,
		task.SourceRSS,
		contentType,
		item.URL,
		"",
	)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout(contentType))
		defer cancel()
		if err := s.router.Run(ctx, t); err != nil {
			slog.Warn("subscription refresh task failed", "url", item.URL, "err", err)
		}
	}()
}

// GET /api/bookmarks — list bookmarks
// POST /api/bookmarks — import bookmark URLs
func (s *server) bookmarks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	userID := defaultUserID()
	switch r.Method {
	case http.MethodGet:
		bookmarks, err := s.st.ListBookmarks(r.Context(), userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bookmarks)
	case http.MethodPost:
		var body bookmarkImportPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		inputs := bookmarkInputsFromPayload(body)
		if len(inputs) == 0 {
			http.Error(w, "missing bookmark url", http.StatusBadRequest)
			return
		}
		result, err := s.st.AddBookmarks(r.Context(), userID, inputs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// PATCH /api/bookmarks/:id — update bookmark metadata
// DELETE /api/bookmarks/:id — delete bookmark
// POST /api/bookmarks/sync — sync pending/selected bookmarks
func (s *server) bookmarkByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "PATCH,DELETE,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	userID := defaultUserID()
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/bookmarks/"), "/")
	if path == "sync" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.syncBookmarks(w, r, userID)
		return
	}
	if path == "" || strings.Contains(path, "/") {
		http.Error(w, "bookmark not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var body bookmarkPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if err := s.st.UpdateBookmark(r.Context(), userID, path, store.BookmarkInput{
			Title:  stringValue(body.Title),
			Folder: stringValue(body.Folder),
			Note:   stringValue(body.Note),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.st.DeleteBookmark(r.Context(), userID, path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) syncBookmarks(w http.ResponseWriter, r *http.Request, userID string) {
	var body bookmarkSyncPayload
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	bookmarks, err := s.st.ListBookmarksForSync(r.Context(), userID, body.IDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	taskIDs := make([]string, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		taskID := fmt.Sprintf("bookmark-%d", time.Now().UnixNano())
		if err := s.st.MarkBookmarkSyncing(r.Context(), userID, bookmark.ID, taskID); err != nil {
			slog.Warn("bookmark sync mark failed", "id", bookmark.ID, "err", err)
			continue
		}
		taskIDs = append(taskIDs, taskID)
		s.runBookmark(userID, bookmark, taskID)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"queued": len(taskIDs), "task_ids": taskIDs})
}

func (s *server) runBookmark(userID string, bookmark store.BookmarkRow, taskID string) {
	contentType := ingest.DetectContentType(bookmark.URL)
	t := task.New(taskID, userID, task.SourceBookmark, contentType, bookmark.URL, "")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout(contentType))
		defer cancel()
		if err := s.router.Run(ctx, t); err != nil {
			_ = s.st.MarkBookmarkFailed(context.Background(), userID, bookmark.ID, err)
			slog.Warn("bookmark sync task failed", "id", bookmark.ID, "err", err)
			return
		}
		_ = s.st.MarkBookmarkSynced(context.Background(), userID, bookmark.ID)
	}()
}

// GET /api/articles?category=&tag=&q=&limit=
func (s *server) articles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := defaultUserID()
	articles, err := s.st.ListArticles(r.Context(), userID, store.ArticleQuery{
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Query:    r.URL.Query().Get("q"),
		Limit:    intQuery(r.URL.Query(), "limit", 50),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(articles)
}

// GET /api/source-items?source_type=&limit=
func (s *server) sourceItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := defaultUserID()
	sourceType := strings.TrimSpace(r.URL.Query().Get("source_type"))
	limit := intQuery(r.URL.Query(), "limit", 80)
	var items []store.SourceItemRow
	var err error
	if boolQuery(r.URL.Query(), "current") {
		items, err = s.st.ListCurrentSourceItems(r.Context(), userID, sourceType, limit)
	} else {
		items, err = s.st.ListSourceItems(r.Context(), userID, sourceType, limit)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// GET /api/knowledge/facets
func (s *server) knowledgeFacets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := defaultUserID()
	facets, err := s.st.KnowledgeFacets(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(facets)
}

// GET /api/search?q=&limit=
func (s *server) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	if s.knowledge == nil {
		http.Error(w, "knowledge service not configured", http.StatusServiceUnavailable)
		return
	}
	userID := defaultUserID()
	result, err := s.knowledge.Search(r.Context(), userID, query, intQuery(r.URL.Query(), "limit", 20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// POST /api/qa { question }
func (s *server) knowledgeQA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.knowledge == nil {
		http.Error(w, "knowledge service not configured", http.StatusServiceUnavailable)
		return
	}
	var body qaPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	userID := defaultUserID()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.knowledge.Answer(ctx, userID, body.Question)
	if err != nil {
		switch {
		case errors.Is(err, knowledgeapp.ErrQuestionRequired):
			http.Error(w, "question is required", http.StatusBadRequest)
		case errors.Is(err, knowledgeapp.ErrLLMNotConfigured):
			http.Error(w, "llm not configured", http.StatusServiceUnavailable)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type subscriptionPayload struct {
	SourceType *string `json:"source_type"`
	FeedURL    *string `json:"feed_url"`
	Title      *string `json:"title"`
	Category   *string `json:"category"`
	Enabled    *bool   `json:"enabled"`
	Account    *string `json:"account"`
	Password   *string `json:"password"`
	Cookie     *string `json:"cookie"`
	AlertHours *int    `json:"alert_hours"`
	NotifyNew  *bool   `json:"notify_new"`
	NotifyDue  *bool   `json:"notify_due"`
}

type bookmarkPayload struct {
	URL    *string `json:"url"`
	Title  *string `json:"title"`
	Folder *string `json:"folder"`
	Note   *string `json:"note"`
}

type bookmarkImportPayload struct {
	URL       string            `json:"url"`
	Text      string            `json:"text"`
	Folder    string            `json:"folder"`
	Bookmarks []bookmarkPayload `json:"bookmarks"`
}

type bookmarkSyncPayload struct {
	IDs []string `json:"ids"`
}

type qaPayload struct {
	Question string `json:"question"`
}

type settingsResponse struct {
	UserID          string                `json:"user_id"`
	Username        string                `json:"username"`
	NotifyChannel   string                `json:"notify_channel"`
	NotifyPolicy    string                `json:"notify_policy"`
	SummaryStyle    string                `json:"summary_style"`
	Language        string                `json:"language"`
	MaxSummaryChars int                   `json:"max_summary_chars"`
	FilterKeywords  []string              `json:"filter_keywords"`
	DailyReport     store.DailyReport     `json:"daily_report"`
	Runtime         settingsRuntime       `json:"runtime"`
	RuntimeConfig   runtimeConfigResponse `json:"runtime_config"`
}

type settingsRuntime struct {
	LLMConfigured          bool `json:"llm_configured"`
	EmbeddingConfigured    bool `json:"embedding_configured"`
	ASRConfigured          bool `json:"asr_configured"`
	TelegramConfigured     bool `json:"telegram_configured"`
	EmailConfigured        bool `json:"email_configured"`
	YTDLPConfigured        bool `json:"yt_dlp_configured"`
	YTDLPCookiesSet        bool `json:"yt_dlp_cookies_set"`
	YTDLPBrowserCookiesSet bool `json:"yt_dlp_browser_cookies_set"`
	PlaywrightConfigured   bool `json:"playwright_configured"`
	FFMPEGConfigured       bool `json:"ffmpeg_configured"`
}

type settingsPatch struct {
	NotifyChannel   *string             `json:"notify_channel"`
	NotifyPolicy    *string             `json:"notify_policy"`
	SummaryStyle    *string             `json:"summary_style"`
	Language        *string             `json:"language"`
	MaxSummaryChars *int                `json:"max_summary_chars"`
	FilterKeywords  *[]string           `json:"filter_keywords"`
	DailyReport     *dailyReportPatch   `json:"daily_report"`
	RuntimeConfig   *runtimeConfigPatch `json:"runtime_config"`
}

type dailyReportPatch struct {
	Enabled  *bool   `json:"enabled"`
	Email    *string `json:"email"`
	Hour     *int    `json:"hour"`
	Timezone *string `json:"timezone"`
	MaxItems *int    `json:"max_items"`
}

func settingsResponseFromStore(settings store.UserSettings, config store.AppConfig) settingsResponse {
	settings = store.NormalizeUserSettings(settings)
	return settingsResponse{
		UserID:          settings.UserID,
		Username:        settings.Username,
		NotifyChannel:   settings.NotifyChannel,
		NotifyPolicy:    settings.ModelPolicy.NotifyPolicy,
		SummaryStyle:    settings.ModelPolicy.SummaryStyle,
		Language:        settings.ModelPolicy.Language,
		MaxSummaryChars: settings.ModelPolicy.MaxSummaryChars,
		FilterKeywords:  settings.FilterKeywords,
		DailyReport:     settings.DailyReport,
		Runtime:         runtimeSettings(config),
		RuntimeConfig:   runtimeConfigResponseFromConfig(config),
	}
}

func applySettingsPatch(current store.UserSettings, patch settingsPatch) (store.UserSettings, error) {
	if patch.NotifyChannel != nil {
		value := strings.TrimSpace(*patch.NotifyChannel)
		if value != "telegram" && value != "email" && value != "none" {
			return store.UserSettings{}, fmt.Errorf("invalid notify_channel")
		}
		current.NotifyChannel = value
	}
	if patch.NotifyPolicy != nil {
		value := strings.TrimSpace(*patch.NotifyPolicy)
		if value != "pass_only" && value != "save_only" {
			return store.UserSettings{}, fmt.Errorf("invalid notify_policy")
		}
		current.ModelPolicy.NotifyPolicy = value
	}
	if patch.SummaryStyle != nil {
		value := strings.TrimSpace(*patch.SummaryStyle)
		if value != "concise" && value != "structured" && value != "actionable" {
			return store.UserSettings{}, fmt.Errorf("invalid summary_style")
		}
		current.ModelPolicy.SummaryStyle = value
	}
	if patch.Language != nil {
		value := strings.TrimSpace(*patch.Language)
		if value != "zh-CN" && value != "en" {
			return store.UserSettings{}, fmt.Errorf("invalid language")
		}
		current.ModelPolicy.Language = value
	}
	if patch.MaxSummaryChars != nil {
		if *patch.MaxSummaryChars < 120 || *patch.MaxSummaryChars > 1000 {
			return store.UserSettings{}, fmt.Errorf("max_summary_chars must be between 120 and 1000")
		}
		current.ModelPolicy.MaxSummaryChars = *patch.MaxSummaryChars
	}
	if patch.FilterKeywords != nil {
		current.FilterKeywords = *patch.FilterKeywords
	}
	if patch.DailyReport != nil {
		report := current.DailyReport
		if patch.DailyReport.Enabled != nil {
			report.Enabled = *patch.DailyReport.Enabled
		}
		if patch.DailyReport.Email != nil {
			value := strings.TrimSpace(*patch.DailyReport.Email)
			if value != "" {
				if _, err := mail.ParseAddress(value); err != nil {
					return store.UserSettings{}, fmt.Errorf("invalid daily_report.email")
				}
			}
			report.Email = value
		}
		if patch.DailyReport.Hour != nil {
			if *patch.DailyReport.Hour < 0 || *patch.DailyReport.Hour > 23 {
				return store.UserSettings{}, fmt.Errorf("daily_report.hour must be between 0 and 23")
			}
			report.Hour = *patch.DailyReport.Hour
		}
		if patch.DailyReport.Timezone != nil {
			value := strings.TrimSpace(*patch.DailyReport.Timezone)
			if value == "" {
				return store.UserSettings{}, fmt.Errorf("daily_report.timezone is required")
			}
			if _, err := time.LoadLocation(value); err != nil {
				return store.UserSettings{}, fmt.Errorf("invalid daily_report.timezone")
			}
			report.Timezone = value
		}
		if patch.DailyReport.MaxItems != nil {
			if *patch.DailyReport.MaxItems < 1 || *patch.DailyReport.MaxItems > 80 {
				return store.UserSettings{}, fmt.Errorf("daily_report.max_items must be between 1 and 80")
			}
			report.MaxItems = *patch.DailyReport.MaxItems
		}
		if report.Enabled && store.DailyReportRecipient(report, current.Username) == "" {
			return store.UserSettings{}, fmt.Errorf("daily_report.email is required when enabled unless username is an email")
		}
		current.DailyReport = report
	}
	return store.NormalizeUserSettings(current), nil
}

func playwrightAvailable() bool {
	driverPath := os.Getenv("PLAYWRIGHT_DRIVER_PATH")
	return executableAvailable(getenv("PLAYWRIGHT_CHROMIUM_EXECUTABLE", "chromium-browser")) &&
		executableAvailable(getenv("PLAYWRIGHT_NODEJS_PATH", "node")) &&
		driverPath != "" &&
		fileAvailable(driverPath+"/package/cli.js")
}

func executableAvailable(name string) bool {
	if strings.Contains(name, "/") {
		info, err := os.Stat(name)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func fileAvailable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func main() {
	ctx := context.Background()

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
	h := &hub{clients: make(map[*websocket.Conn]struct{})}

	llmClient := &runtimeconfig.LLM{Store: st}
	embeddingClient := &runtimeconfig.Embedder{Store: st}
	notifier := &runtimeconfig.Notifier{Store: st}

	srv := &server{st: st, hub: h, knowledge: knowledgeapp.NewService(st, llmClient, embeddingClient)}
	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, notifier, srv.onStatus),
		pipeline.NewVideo(fetcher.NewVideo(), llmClient, llmClient, st, notifier, srv.onStatus),
	)
	if err != nil {
		slog.Error("router init", "err", err)
		os.Exit(1)
	}
	srv.router = router

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			return
		}
		switch r.Method {
		case http.MethodPost:
			srv.createTask(w, r)
		case http.MethodGet:
			srv.listTasks(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	apiMux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			return
		}
		srv.retryTask(w, r)
	})
	apiMux.HandleFunc("/api/subscriptions", srv.subscriptions)
	apiMux.HandleFunc("/api/subscriptions/", srv.subscriptionByID)
	apiMux.HandleFunc("/api/bookmarks", srv.bookmarks)
	apiMux.HandleFunc("/api/bookmarks/", srv.bookmarkByID)
	apiMux.HandleFunc("/api/articles", srv.articles)
	apiMux.HandleFunc("/api/source-items", srv.sourceItems)
	apiMux.HandleFunc("/api/knowledge/facets", srv.knowledgeFacets)
	apiMux.HandleFunc("/api/search", srv.searchKnowledge)
	apiMux.HandleFunc("/api/qa", srv.knowledgeQA)
	apiMux.HandleFunc("/api/settings", srv.settings)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/status", srv.authStatus)
	mux.HandleFunc("/api/auth/setup", srv.authSetup)
	mux.HandleFunc("/api/auth/login", srv.authLogin)
	mux.HandleFunc("/api/auth/logout", srv.authLogout)
	mux.Handle("/api/", srv.requireAuth(apiMux))
	mux.Handle("/ws", srv.requireAuth(http.HandlerFunc(srv.wsHandler)))
	mux.Handle("/", http.FileServer(http.Dir(getenv("WEB_DIR", "./web/dist"))))

	addr := ":" + getenv("PORT", "8080")
	slog.Info("api listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
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

func taskTimeout(contentType task.ContentType) time.Duration {
	if contentType == task.ContentVideo {
		return 30 * time.Minute
	}
	return 5 * time.Minute
}

func subscriptionSourceType(body subscriptionPayload) string {
	if body.SourceType == nil {
		return "rss"
	}
	value := strings.TrimSpace(*body.SourceType)
	if value == "" {
		return "rss"
	}
	return value
}

func chaoxingInputFromPayload(body subscriptionPayload, existing *store.SourceSubscriptionRow) store.ChaoxingSubscriptionInput {
	input := store.ChaoxingSubscriptionInput{
		Title:      stringValue(body.Title),
		Category:   stringValue(body.Category),
		Account:    stringValue(body.Account),
		Password:   rawStringValue(body.Password),
		Cookie:     rawStringValue(body.Cookie),
		NotifyNew:  body.NotifyNew,
		NotifyDue:  body.NotifyDue,
		Enabled:    body.Enabled,
		AlertHours: 24,
	}
	if body.AlertHours != nil {
		input.AlertHours = *body.AlertHours
	} else if existing != nil && existing.AlertHours > 0 {
		input.AlertHours = existing.AlertHours
	}
	if existing != nil {
		if body.Title == nil {
			input.Title = existing.Title
		}
		if body.Category == nil {
			input.Category = existing.Category
		}
		if body.Account == nil {
			input.Account = existing.Account
		}
		if body.NotifyNew == nil {
			value := existing.NotifyNew
			input.NotifyNew = &value
		}
		if body.NotifyDue == nil {
			value := existing.NotifyDue
			input.NotifyDue = &value
		}
		if body.Enabled == nil {
			value := existing.Enabled
			input.Enabled = &value
		}
	}
	return input
}

func statusFromStoreError(err error) int {
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound
	}
	message := err.Error()
	if strings.Contains(message, "required") || strings.Contains(message, "must be between") || strings.Contains(message, "invalid") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func normalizeFeedURL(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("empty feed url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported feed url scheme")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("missing feed url host")
	}
	return parsed.String(), nil
}

func bookmarkInputsFromPayload(body bookmarkImportPayload) []store.BookmarkInput {
	inputs := make([]store.BookmarkInput, 0, len(body.Bookmarks)+8)
	fallbackFolder := strings.TrimSpace(body.Folder)
	if strings.TrimSpace(body.URL) != "" {
		if normalized, err := ingest.NormalizeURL(body.URL); err == nil {
			inputs = append(inputs, store.BookmarkInput{URL: normalized, Folder: fallbackFolder})
		}
	}
	for _, rawURL := range ingest.ExtractURLs(body.Text) {
		inputs = append(inputs, store.BookmarkInput{URL: rawURL, Folder: fallbackFolder})
	}
	for _, bookmark := range body.Bookmarks {
		if bookmark.URL == nil {
			continue
		}
		normalized, err := ingest.NormalizeURL(*bookmark.URL)
		if err != nil {
			continue
		}
		folder := stringValue(bookmark.Folder)
		if folder == "" {
			folder = fallbackFolder
		}
		inputs = append(inputs, store.BookmarkInput{
			URL:    normalized,
			Title:  stringValue(bookmark.Title),
			Folder: folder,
			Note:   stringValue(bookmark.Note),
		})
	}
	out := make([]store.BookmarkInput, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if _, ok := seen[input.URL]; ok {
			continue
		}
		seen[input.URL] = struct{}{}
		out = append(out, input)
	}
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func rawStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intQuery(values url.Values, key string, fallback int) int {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func boolQuery(values url.Values, key string) bool {
	raw := strings.TrimSpace(strings.ToLower(values.Get(key)))
	return raw == "1" || raw == "true" || raw == "yes"
}
