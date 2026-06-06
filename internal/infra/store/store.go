package store

import (
	"context"
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements pipeline.Store backed by PostgreSQL.
type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) SaveTaskState(ctx context.Context, t *task.Task) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO tasks (id, user_id, source, content_type, url, raw_content,
		                   status, filter_decision, summary, error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			raw_content      = EXCLUDED.raw_content,
			status          = EXCLUDED.status,
			filter_decision = EXCLUDED.filter_decision,
			summary         = EXCLUDED.summary,
			error           = EXCLUDED.error,
			updated_at      = EXCLUDED.updated_at`,
		t.ID, t.UserID, string(t.Source), string(t.ContentType),
		t.URL, t.RawContent(),
		string(t.Status()), string(t.FilterDecision()),
		t.Summary(), t.Error(),
		t.CreatedAt(), t.UpdatedAt(),
	)
	return err
}

func (s *Store) AppendStep(ctx context.Context, taskID string, step task.Step) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO task_steps (task_id, label, status, detail, duration_ms, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		taskID, step.Label, string(step.Status), step.Detail,
		step.Duration.Milliseconds(), step.At,
	)
	return err
}

func (s *Store) SaveKnowledgeItem(ctx context.Context, t *task.Task) error {
	hash := contentHash(t.URL)
	_, err := s.db.Exec(ctx, `
		INSERT INTO articles (id, user_id, task_id, url, url_hash, source, content, summary, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		ON CONFLICT (user_id, url_hash) WHERE url_hash IS NOT NULL DO UPDATE SET
			summary    = EXCLUDED.summary,
			content    = EXCLUDED.content`,
		t.ID, t.UserID, t.ID, t.URL, hash,
		string(t.Source), t.RawContent(), t.Summary(),
	)
	return err
}

func (s *Store) IsDuplicate(ctx context.Context, url string) (bool, error) {
	if url == "" {
		return false, nil
	}
	hash := contentHash(url)
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM articles WHERE url_hash = $1)`, hash,
	).Scan(&exists)
	return exists, err
}

func contentHash(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%x", xxhash.Sum64String(s))
}

// TaskRow is a flat representation for API responses.
type TaskRow struct {
	ID             string    `json:"id"`
	Source         string    `json:"source"`
	ContentType    string    `json:"content_type"`
	URL            string    `json:"url"`
	Status         string    `json:"status"`
	FilterDecision string    `json:"filter_decision"`
	Summary        string    `json:"summary"`
	Error          string    `json:"error"`
	CreatedAt      string    `json:"created_at"`
	Steps          []StepRow `json:"steps"`
}

type StepRow struct {
	Label      string `json:"label"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"duration_ms"`
}

func (s *Store) ListTasks(ctx context.Context, userID string, limit int) ([]TaskRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source, content_type, url, status, filter_decision,
		       summary, error, created_at
		FROM tasks WHERE user_id = $1
		ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskRow
	for rows.Next() {
		var t TaskRow
		var createdAt interface{}
		if err := rows.Scan(&t.ID, &t.Source, &t.ContentType, &t.URL,
			&t.Status, &t.FilterDecision, &t.Summary, &t.Error, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = fmt.Sprintf("%v", createdAt)
		t.Steps, _ = s.listSteps(ctx, t.ID)
		tasks = append(tasks, t)
	}
	if tasks == nil {
		tasks = []TaskRow{}
	}
	return tasks, nil
}

func (s *Store) GetTask(ctx context.Context, id string) (*TaskRow, error) {
	var t TaskRow
	var createdAt interface{}
	err := s.db.QueryRow(ctx, `
		SELECT id, source, content_type, url, status, filter_decision,
		       summary, error, created_at
		FROM tasks WHERE id = $1`, id).Scan(
		&t.ID, &t.Source, &t.ContentType, &t.URL,
		&t.Status, &t.FilterDecision, &t.Summary, &t.Error, &createdAt)
	if err != nil {
		return nil, err
	}
	t.CreatedAt = fmt.Sprintf("%v", createdAt)
	return &t, nil
}

func (s *Store) listSteps(ctx context.Context, taskID string) ([]StepRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT label, status, detail, duration_ms FROM task_steps
		WHERE task_id = $1 ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []StepRow
	for rows.Next() {
		var st StepRow
		if err := rows.Scan(&st.Label, &st.Status, &st.Detail, &st.DurationMs); err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// ── Subscription ─────────────────────────────────────────────────────────────

type SubscriptionRow struct {
	ID            string      `json:"id"`
	UserID        string      `json:"user_id"`
	SourceType    string      `json:"source_type"`
	FeedURL       string      `json:"feed_url"`
	LastFetchedAt interface{} `json:"last_fetched_at"`
	Enabled       bool        `json:"enabled"`
}

func (s *Store) ListRSSSubscriptions(ctx context.Context) ([]SubscriptionRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, source_type, config->>'feed_url', last_fetched_at, enabled
		FROM subscriptions WHERE source_type = 'rss' AND enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []SubscriptionRow
	for rows.Next() {
		var sub SubscriptionRow
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.SourceType,
			&sub.FeedURL, &sub.LastFetchedAt, &sub.Enabled); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func (s *Store) UpdateLastFetched(ctx context.Context, subID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE subscriptions SET last_fetched_at = NOW() WHERE id = $1`, subID)
	return err
}

func (s *Store) AddRSSSubscription(ctx context.Context, userID, feedURL string) (string, error) {
	id := fmt.Sprintf("sub-%d", time.Now().UnixMilli())
	_, err := s.db.Exec(ctx, `
		INSERT INTO subscriptions (id, user_id, source_type, config, enabled)
		VALUES ($1, $2, 'rss', jsonb_build_object('feed_url', $3::text), true)
		ON CONFLICT DO NOTHING`,
		id, userID, feedURL)
	return id, err
}

// Verify Store satisfies pipeline.Store at compile time.
var _ pipeline.Store = (*Store)(nil)
