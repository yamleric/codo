package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/codo/codo/internal/application/ingest"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/codo/codo/internal/infra/notify"
	"github.com/codo/codo/internal/infra/sources"
	"github.com/codo/codo/internal/infra/store"
	"github.com/gorilla/websocket"
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
	st     *store.Store
	router *pipeline.Router
	hub    *hub
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

	userID := getenv("DEFAULT_USER_ID", "demo-user")
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
	userID := getenv("DEFAULT_USER_ID", "demo-user")
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

	userID := getenv("DEFAULT_USER_ID", "demo-user")
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

	userID := getenv("DEFAULT_USER_ID", "demo-user")
	current, err := s.st.GetUserSettings(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponseFromStore(current))
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settingsResponseFromStore(updated))
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

// GET /api/subscriptions — list RSS subscriptions
// POST /api/subscriptions — add RSS subscription { feed_url, title?, category? }
func (s *server) subscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	if r.Method == http.MethodOptions {
		return
	}
	userID := getenv("DEFAULT_USER_ID", "demo-user")
	switch r.Method {
	case http.MethodGet:
		subs, err := s.st.ListRSSSubscriptions(r.Context(), userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)
	case http.MethodPost:
		var body subscriptionPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FeedURL == nil {
			http.Error(w, "invalid feed_url", http.StatusBadRequest)
			return
		}
		feedURL, err := normalizeFeedURL(*body.FeedURL)
		if err != nil {
			http.Error(w, "invalid feed_url", http.StatusBadRequest)
			return
		}
		id, err := s.st.AddRSSSubscription(r.Context(), userID, feedURL, stringValue(body.Title), stringValue(body.Category))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// PATCH /api/subscriptions/:id — update RSS subscription
// DELETE /api/subscriptions/:id — delete RSS subscription
// POST /api/subscriptions/:id/refresh — fetch new RSS items now
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
	userID := getenv("DEFAULT_USER_ID", "demo-user")

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
		existing, err := s.st.GetRSSSubscription(r.Context(), userID, id)
		if err != nil {
			http.Error(w, "subscription not found", http.StatusNotFound)
			return
		}
		var body subscriptionPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
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
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.st.DeleteRSSSubscription(r.Context(), userID, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) refreshSubscription(w http.ResponseWriter, r *http.Request, userID, id string) {
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

type subscriptionPayload struct {
	FeedURL  *string `json:"feed_url"`
	Title    *string `json:"title"`
	Category *string `json:"category"`
	Enabled  *bool   `json:"enabled"`
}

type settingsResponse struct {
	UserID          string          `json:"user_id"`
	NotifyChannel   string          `json:"notify_channel"`
	NotifyPolicy    string          `json:"notify_policy"`
	SummaryStyle    string          `json:"summary_style"`
	Language        string          `json:"language"`
	MaxSummaryChars int             `json:"max_summary_chars"`
	FilterKeywords  []string        `json:"filter_keywords"`
	Runtime         settingsRuntime `json:"runtime"`
}

type settingsRuntime struct {
	LLMConfigured          bool `json:"llm_configured"`
	ASRConfigured          bool `json:"asr_configured"`
	TelegramConfigured     bool `json:"telegram_configured"`
	YTDLPConfigured        bool `json:"yt_dlp_configured"`
	YTDLPCookiesSet        bool `json:"yt_dlp_cookies_set"`
	YTDLPBrowserCookiesSet bool `json:"yt_dlp_browser_cookies_set"`
	PlaywrightConfigured   bool `json:"playwright_configured"`
	FFMPEGConfigured       bool `json:"ffmpeg_configured"`
}

type settingsPatch struct {
	NotifyChannel   *string   `json:"notify_channel"`
	NotifyPolicy    *string   `json:"notify_policy"`
	SummaryStyle    *string   `json:"summary_style"`
	Language        *string   `json:"language"`
	MaxSummaryChars *int      `json:"max_summary_chars"`
	FilterKeywords  *[]string `json:"filter_keywords"`
}

func settingsResponseFromStore(settings store.UserSettings) settingsResponse {
	settings = store.NormalizeUserSettings(settings)
	return settingsResponse{
		UserID:          settings.UserID,
		NotifyChannel:   settings.NotifyChannel,
		NotifyPolicy:    settings.ModelPolicy.NotifyPolicy,
		SummaryStyle:    settings.ModelPolicy.SummaryStyle,
		Language:        settings.ModelPolicy.Language,
		MaxSummaryChars: settings.ModelPolicy.MaxSummaryChars,
		FilterKeywords:  settings.FilterKeywords,
		Runtime:         runtimeSettings(),
	}
}

func applySettingsPatch(current store.UserSettings, patch settingsPatch) (store.UserSettings, error) {
	if patch.NotifyChannel != nil {
		value := strings.TrimSpace(*patch.NotifyChannel)
		if value != "telegram" && value != "none" {
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
	return store.NormalizeUserSettings(current), nil
}

func runtimeSettings() settingsRuntime {
	_, ytDLPErr := exec.LookPath(getenv("YTDLP_BIN", "yt-dlp"))
	_, ffmpegErr := exec.LookPath(getenv("FFMPEG_BIN", "ffmpeg"))
	return settingsRuntime{
		LLMConfigured:          os.Getenv("LLM_API_KEY") != "",
		ASRConfigured:          os.Getenv("ASR_BASE_URL") != "" && os.Getenv("ASR_API_KEY") != "",
		TelegramConfigured:     os.Getenv("TELEGRAM_TOKEN") != "",
		YTDLPConfigured:        ytDLPErr == nil,
		YTDLPCookiesSet:        os.Getenv("YTDLP_COOKIES_FILE") != "" || os.Getenv("YTDLP_COOKIES_FROM_BROWSER") != "",
		YTDLPBrowserCookiesSet: os.Getenv("YTDLP_COOKIES_FROM_BROWSER") != "",
		PlaywrightConfigured:   playwrightAvailable(),
		FFMPEGConfigured:       ffmpegErr == nil,
	}
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

	llmClient := llm.NewClient(llm.Config{
		BaseURL:     getenv("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:      getenv("LLM_API_KEY", ""),
		Model:       getenv("LLM_MODEL", "gpt-4o-mini"),
		Preferences: st,
	})

	var notifier pipeline.Notifier = &logNotifier{}
	if tg, err := notify.NewTelegram(); err == nil {
		notifier = tg
	}

	srv := &server{st: st, hub: h}
	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, notifier, srv.onStatus),
		pipeline.NewVideo(fetcher.NewVideo(), llmClient, llmClient, st, notifier, srv.onStatus),
	)
	if err != nil {
		slog.Error("router init", "err", err)
		os.Exit(1)
	}
	srv.router = router

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("/api/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			return
		}
		srv.retryTask(w, r)
	})
	mux.HandleFunc("/api/subscriptions", srv.subscriptions)
	mux.HandleFunc("/api/subscriptions/", srv.subscriptionByID)
	mux.HandleFunc("/api/settings", srv.settings)
	mux.HandleFunc("/ws", srv.wsHandler)
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

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
