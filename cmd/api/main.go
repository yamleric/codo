package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/db"
	"github.com/codo/codo/internal/infra/fetcher"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/codo/codo/internal/infra/notify"
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

	userID := getenv("DEFAULT_USER_ID", "demo-user")
	id := fmt.Sprintf("task-%d", time.Now().UnixMilli())
	t := task.New(id, userID, task.SourceManual, task.ContentWebPage, body.URL, "")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_ = s.router.Run(ctx, t)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": newID})
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
// POST /api/subscriptions — add RSS subscription { feed_url: string }
func (s *server) subscriptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		return
	}
	userID := getenv("DEFAULT_USER_ID", "demo-user")
	switch r.Method {
	case http.MethodGet:
		subs, err := s.st.ListRSSSubscriptions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)
	case http.MethodPost:
		var body struct {
			FeedURL string `json:"feed_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.FeedURL) == "" {
			http.Error(w, "invalid feed_url", http.StatusBadRequest)
			return
		}
		id, err := s.st.AddRSSSubscription(r.Context(), userID, body.FeedURL)
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

func main() {
	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}

	st := store.New(pool)
	h := &hub{clients: make(map[*websocket.Conn]struct{})}

	llmClient := llm.NewClient(llm.Config{
		BaseURL: getenv("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:  getenv("LLM_API_KEY", ""),
		Model:   getenv("LLM_MODEL", "gpt-4o-mini"),
	})

	var notifier pipeline.Notifier = &logNotifier{}
	if tg, err := notify.NewTelegram(); err == nil {
		notifier = tg
	}

	srv := &server{st: st, hub: h}
	router, err := pipeline.NewRouter(
		pipeline.NewWebPage(fetcher.NewHTTP(), llmClient, llmClient, st, notifier, srv.onStatus),
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
