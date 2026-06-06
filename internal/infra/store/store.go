package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
	"github.com/codo/codo/internal/infra/llm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	if err := s.ensureUser(ctx, t.UserID); err != nil {
		return err
	}
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

// ── User settings ───────────────────────────────────────────────────────────

type UserSettings struct {
	UserID         string          `json:"user_id"`
	NotifyChannel  string          `json:"notify_channel"`
	FilterKeywords []string        `json:"filter_keywords"`
	ModelPolicy    UserModelPolicy `json:"model_policy"`
}

type UserModelPolicy struct {
	SummaryStyle    string `json:"summary_style"`
	Language        string `json:"language"`
	MaxSummaryChars int    `json:"max_summary_chars"`
	NotifyPolicy    string `json:"notify_policy"`
}

func DefaultUserModelPolicy() UserModelPolicy {
	return UserModelPolicy{
		SummaryStyle:    "concise",
		Language:        "zh-CN",
		MaxSummaryChars: 300,
		NotifyPolicy:    "pass_only",
	}
}

func NormalizeUserSettings(settings UserSettings) UserSettings {
	switch settings.NotifyChannel {
	case "telegram", "none":
	default:
		settings.NotifyChannel = "telegram"
	}
	settings.FilterKeywords = normalizeKeywords(settings.FilterKeywords)
	settings.ModelPolicy = NormalizeUserModelPolicy(settings.ModelPolicy)
	return settings
}

func NormalizeUserModelPolicy(policy UserModelPolicy) UserModelPolicy {
	defaults := DefaultUserModelPolicy()
	switch policy.SummaryStyle {
	case "concise", "structured", "actionable":
	default:
		policy.SummaryStyle = defaults.SummaryStyle
	}
	switch policy.Language {
	case "zh-CN", "en":
	default:
		policy.Language = defaults.Language
	}
	if policy.MaxSummaryChars < 120 {
		policy.MaxSummaryChars = defaults.MaxSummaryChars
	}
	if policy.MaxSummaryChars > 1000 {
		policy.MaxSummaryChars = 1000
	}
	switch policy.NotifyPolicy {
	case "pass_only", "save_only":
	default:
		policy.NotifyPolicy = defaults.NotifyPolicy
	}
	return policy
}

func (s *Store) GetUserSettings(ctx context.Context, userID string) (UserSettings, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return UserSettings{}, err
	}

	settings := UserSettings{}
	var rawPolicy string
	err := s.db.QueryRow(ctx, `
		SELECT id,
		       COALESCE(NULLIF(notify_channel, ''), 'telegram'),
		       COALESCE(filter_keywords, '{}'::text[]),
		       COALESCE(model_policy, '{}'::jsonb)::text
		FROM users
		WHERE id = $1`, userID).Scan(
		&settings.UserID,
		&settings.NotifyChannel,
		&settings.FilterKeywords,
		&rawPolicy,
	)
	if err != nil {
		return UserSettings{}, err
	}
	if strings.TrimSpace(rawPolicy) != "" {
		if err := json.Unmarshal([]byte(rawPolicy), &settings.ModelPolicy); err != nil {
			return UserSettings{}, fmt.Errorf("parse user model policy: %w", err)
		}
	}
	return NormalizeUserSettings(settings), nil
}

func (s *Store) UpdateUserSettings(ctx context.Context, settings UserSettings) error {
	if err := s.ensureUser(ctx, settings.UserID); err != nil {
		return err
	}
	settings = NormalizeUserSettings(settings)
	policyJSON, err := json.Marshal(settings.ModelPolicy)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		UPDATE users
		SET notify_channel = $2,
		    filter_keywords = $3,
		    model_policy = $4::jsonb
		WHERE id = $1`,
		settings.UserID,
		settings.NotifyChannel,
		settings.FilterKeywords,
		string(policyJSON),
	)
	return err
}

func (s *Store) GetPipelineSettings(ctx context.Context, userID string) (pipeline.UserSettings, error) {
	settings, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		return pipeline.UserSettings{}, err
	}
	return pipeline.UserSettings{
		NotifyChannel: settings.NotifyChannel,
		NotifyPolicy:  settings.ModelPolicy.NotifyPolicy,
	}, nil
}

func (s *Store) GetLLMPreferences(ctx context.Context, userID string) (llm.UserPreferences, error) {
	settings, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		return llm.UserPreferences{}, err
	}
	return llm.UserPreferences{
		FilterKeywords:  settings.FilterKeywords,
		SummaryStyle:    settings.ModelPolicy.SummaryStyle,
		Language:        settings.ModelPolicy.Language,
		MaxSummaryChars: settings.ModelPolicy.MaxSummaryChars,
	}, nil
}

func normalizeKeywords(keywords []string) []string {
	out := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		runes := []rune(keyword)
		if len(runes) > 40 {
			keyword = string(runes[:40])
		}
		key := strings.ToLower(keyword)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, keyword)
		if len(out) >= 32 {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

// ── Subscription ─────────────────────────────────────────────────────────────

type SubscriptionRow struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	SourceType    string     `json:"source_type"`
	FeedURL       string     `json:"feed_url"`
	Title         string     `json:"title"`
	Category      string     `json:"category"`
	LastFetchedAt *time.Time `json:"last_fetched_at"`
	LastError     string     `json:"last_error"`
	LastErrorAt   *time.Time `json:"last_error_at"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (s *Store) ListRSSSubscriptions(ctx context.Context, userID string) ([]SubscriptionRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, source_type,
		       COALESCE(config->>'feed_url', ''),
		       COALESCE(config->>'title', ''),
		       COALESCE(config->>'category', ''),
		       last_fetched_at,
		       COALESCE(config->>'last_error', ''),
		       NULLIF(config->>'last_error_at', '')::timestamptz,
		       enabled,
		       created_at
		FROM subscriptions
		WHERE source_type = 'rss' AND user_id = $1
		ORDER BY enabled DESC, COALESCE(config->>'category', '') ASC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []SubscriptionRow
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		subs = []SubscriptionRow{}
	}
	return subs, nil
}

func (s *Store) ListActiveRSSSubscriptions(ctx context.Context) ([]SubscriptionRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, source_type,
		       COALESCE(config->>'feed_url', ''),
		       COALESCE(config->>'title', ''),
		       COALESCE(config->>'category', ''),
		       last_fetched_at,
		       COALESCE(config->>'last_error', ''),
		       NULLIF(config->>'last_error_at', '')::timestamptz,
		       enabled,
		       created_at
		FROM subscriptions
		WHERE source_type = 'rss' AND enabled = true
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []SubscriptionRow
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		subs = []SubscriptionRow{}
	}
	return subs, nil
}

func (s *Store) GetRSSSubscription(ctx context.Context, userID, id string) (*SubscriptionRow, error) {
	sub, err := scanSubscription(s.db.QueryRow(ctx, `
		SELECT id, user_id, source_type,
		       COALESCE(config->>'feed_url', ''),
		       COALESCE(config->>'title', ''),
		       COALESCE(config->>'category', ''),
		       last_fetched_at,
		       COALESCE(config->>'last_error', ''),
		       NULLIF(config->>'last_error_at', '')::timestamptz,
		       enabled,
		       created_at
		FROM subscriptions
		WHERE source_type = 'rss' AND user_id = $1 AND id = $2`, userID, id))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) UpdateLastFetched(ctx context.Context, subID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE subscriptions
		 SET last_fetched_at = NOW(),
		     config = config - 'last_error' - 'last_error_at'
		 WHERE id = $1`, subID)
	return err
}

func (s *Store) RecordRSSFetchFailure(ctx context.Context, subID string, fetchErr error) error {
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE subscriptions
		SET config = jsonb_set(
		              jsonb_set(config, '{last_error}', to_jsonb($2::text), true),
		              '{last_error_at}', to_jsonb(NOW()::text), true
		            )
		WHERE id = $1`, subID, truncateText(msg, 420))
	return err
}

func (s *Store) AddRSSSubscription(ctx context.Context, userID, feedURL, title, category string) (string, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return "", err
	}
	if existingID, err := s.findRSSSubscriptionID(ctx, userID, feedURL); err != nil {
		return "", err
	} else if existingID != "" {
		_, err := s.db.Exec(ctx, `
			UPDATE subscriptions
			SET enabled = true,
			    config = jsonb_set(
			        jsonb_set(config, '{title}', to_jsonb($3::text), true),
			        '{category}', to_jsonb($4::text), true
			    )
			WHERE id = $1 AND user_id = $2`,
			existingID, userID, strings.TrimSpace(title), strings.TrimSpace(category))
		return existingID, err
	}

	id := fmt.Sprintf("sub-%d", time.Now().UnixMilli())
	_, err := s.db.Exec(ctx, `
		INSERT INTO subscriptions (id, user_id, source_type, config, enabled)
		VALUES ($1, $2, 'rss',
		        jsonb_build_object(
		          'feed_url', $3::text,
		          'title', $4::text,
		          'category', $5::text
		        ), true)
		ON CONFLICT DO NOTHING`,
		id, userID, feedURL, strings.TrimSpace(title), strings.TrimSpace(category))
	return id, err
}

func (s *Store) UpdateRSSSubscription(ctx context.Context, userID, id, feedURL, title, category string, enabled bool) error {
	_, err := s.db.Exec(ctx, `
		UPDATE subscriptions
		SET enabled = $6,
		    config = jsonb_set(
		        jsonb_set(
		          jsonb_set(config, '{feed_url}', to_jsonb($3::text), true),
		          '{title}', to_jsonb($4::text), true
		        ),
		        '{category}', to_jsonb($5::text), true
		    )
		WHERE id = $1 AND user_id = $2 AND source_type = 'rss'`,
		id, userID, feedURL, strings.TrimSpace(title), strings.TrimSpace(category), enabled)
	return err
}

func (s *Store) DeleteRSSSubscription(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM subscriptions WHERE id = $1 AND user_id = $2 AND source_type = 'rss'`,
		id, userID)
	return err
}

func (s *Store) findRSSSubscriptionID(ctx context.Context, userID, feedURL string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM subscriptions
		WHERE user_id = $1 AND source_type = 'rss' AND config->>'feed_url' = $2
		LIMIT 1`, userID, feedURL).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

func (s *Store) ensureUser(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO users (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING`, userID)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row scanner) (SubscriptionRow, error) {
	var sub SubscriptionRow
	var lastFetched pgtype.Timestamptz
	var lastErrorAt pgtype.Timestamptz
	err := row.Scan(&sub.ID, &sub.UserID, &sub.SourceType,
		&sub.FeedURL, &sub.Title, &sub.Category,
		&lastFetched, &sub.LastError, &lastErrorAt,
		&sub.Enabled, &sub.CreatedAt)
	if err != nil {
		return sub, err
	}
	if lastFetched.Valid {
		t := lastFetched.Time
		sub.LastFetchedAt = &t
	}
	if lastErrorAt.Valid {
		t := lastErrorAt.Time
		sub.LastErrorAt = &t
	}
	return sub, nil
}

func truncateText(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// Verify Store satisfies pipeline.Store at compile time.
var _ pipeline.Store = (*Store)(nil)
var _ pipeline.UserSettingsProvider = (*Store)(nil)
var _ llm.UserPreferencesProvider = (*Store)(nil)
