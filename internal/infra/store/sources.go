package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SourceSubscriptionRow struct {
	ID                 string     `json:"id"`
	UserID             string     `json:"user_id"`
	SourceType         string     `json:"source_type"`
	FeedURL            string     `json:"feed_url"`
	Title              string     `json:"title"`
	Category           string     `json:"category"`
	Account            string     `json:"account"`
	Provider           string     `json:"provider"`
	Host               string     `json:"host"`
	Port               int        `json:"port"`
	Mailbox            string     `json:"mailbox"`
	PasswordConfigured bool       `json:"password_configured"`
	CookieConfigured   bool       `json:"cookie_configured"`
	AlertHours         int        `json:"alert_hours"`
	NotifyNew          bool       `json:"notify_new"`
	NotifyDue          bool       `json:"notify_due"`
	NotifyImportant    bool       `json:"notify_important"`
	SyncUnreadOnly     bool       `json:"sync_unread_only"`
	SinceDays          int        `json:"since_days"`
	MaxMessages        int        `json:"max_messages"`
	LastFetchedAt      *time.Time `json:"last_fetched_at"`
	LastError          string     `json:"last_error"`
	LastErrorAt        *time.Time `json:"last_error_at"`
	Enabled            bool       `json:"enabled"`
	CreatedAt          time.Time  `json:"created_at"`
}

type ChaoxingSubscription struct {
	ID            string
	UserID        string
	Title         string
	Category      string
	Account       string
	Password      string
	Cookie        string
	AlertHours    int
	NotifyNew     bool
	NotifyDue     bool
	LastFetchedAt *time.Time
	LastError     string
	LastErrorAt   *time.Time
	Enabled       bool
	CreatedAt     time.Time
}

type ChaoxingSubscriptionInput struct {
	Title      string
	Category   string
	Account    string
	Password   string
	Cookie     string
	AlertHours int
	NotifyNew  *bool
	NotifyDue  *bool
	Enabled    *bool
}

type EmailSubscription struct {
	ID              string
	UserID          string
	Title           string
	Category        string
	Account         string
	Password        string
	Provider        string
	Host            string
	Port            int
	Mailbox         string
	SinceDays       int
	MaxMessages     int
	NotifyImportant bool
	SyncUnreadOnly  bool
	LastFetchedAt   *time.Time
	LastError       string
	LastErrorAt     *time.Time
	Enabled         bool
	CreatedAt       time.Time
}

type EmailSubscriptionInput struct {
	Title           string
	Category        string
	Account         string
	Password        string
	Provider        string
	Host            string
	Port            int
	Mailbox         string
	SinceDays       int
	MaxMessages     int
	NotifyImportant *bool
	SyncUnreadOnly  *bool
	Enabled         *bool
}

type SourceItemRow struct {
	ID             string         `json:"id"`
	UserID         string         `json:"user_id"`
	SubscriptionID string         `json:"subscription_id"`
	SourceType     string         `json:"source_type"`
	ItemType       string         `json:"item_type"`
	ExternalID     string         `json:"external_id"`
	Course         string         `json:"course"`
	Title          string         `json:"title"`
	Status         string         `json:"status"`
	URL            string         `json:"url"`
	DueAt          *time.Time     `json:"due_at"`
	Payload        map[string]any `json:"payload"`
	FirstSeenAt    time.Time      `json:"first_seen_at"`
	LastSeenAt     time.Time      `json:"last_seen_at"`
	NewNotifiedAt  *time.Time     `json:"new_notified_at"`
	DueNotifiedAt  *time.Time     `json:"due_notified_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type SourceItemInput struct {
	UserID         string
	SubscriptionID string
	SourceType     string
	ItemType       string
	ExternalID     string
	Course         string
	Title          string
	Status         string
	URL            string
	DueAt          *time.Time
	Payload        map[string]any
}

type SourceItemChange struct {
	Created       bool
	StatusChanged bool
	DueChanged    bool
}

func (s *Store) ListSubscriptions(ctx context.Context, userID string) ([]SourceSubscriptionRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, source_type, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE user_id = $1 AND source_type IN ('rss', 'chaoxing', 'email')
		ORDER BY enabled DESC, source_type ASC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []SourceSubscriptionRow
	for rows.Next() {
		sub, err := scanSourceSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		return []SourceSubscriptionRow{}, nil
	}
	return subs, rows.Err()
}

func (s *Store) GetSourceSubscription(ctx context.Context, userID, id string) (*SourceSubscriptionRow, error) {
	sub, err := scanSourceSubscription(s.db.QueryRow(ctx, `
		SELECT id, user_id, source_type, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE user_id = $1 AND id = $2`, userID, id))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) AddChaoxingSubscription(ctx context.Context, userID string, input ChaoxingSubscriptionInput) (string, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return "", err
	}
	input = normalizeChaoxingInput(input, nil)
	if err := validateChaoxingInput(input); err != nil {
		return "", err
	}
	if existingID, err := s.findChaoxingSubscriptionID(ctx, userID, input.Account); err != nil {
		return "", err
	} else if existingID != "" {
		enabled := true
		input.Enabled = &enabled
		return existingID, s.UpdateChaoxingSubscription(ctx, userID, existingID, input)
	}

	id := fmt.Sprintf("chaoxing-%d", time.Now().UnixMilli())
	config, err := marshalChaoxingConfig(input)
	if err != nil {
		return "", err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO subscriptions (id, user_id, source_type, config, enabled)
		VALUES ($1, $2, 'chaoxing', $3::jsonb, $4)`,
		id, userID, config, enabled)
	return id, err
}

func (s *Store) UpdateChaoxingSubscription(ctx context.Context, userID, id string, input ChaoxingSubscriptionInput) error {
	existing, err := s.GetChaoxingSubscription(ctx, userID, id)
	if err != nil {
		return err
	}
	input = normalizeChaoxingInput(input, existing)
	if err := validateChaoxingInput(input); err != nil {
		return err
	}
	config, err := marshalChaoxingConfig(input)
	if err != nil {
		return err
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE subscriptions
		SET config = $3::jsonb, enabled = $4
		WHERE id = $1 AND user_id = $2 AND source_type = 'chaoxing'`,
		id, userID, config, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetChaoxingSubscription(ctx context.Context, userID, id string) (*ChaoxingSubscription, error) {
	sub, err := scanChaoxingSubscription(s.db.QueryRow(ctx, `
		SELECT id, user_id, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE user_id = $1 AND id = $2 AND source_type = 'chaoxing'`, userID, id))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) ListActiveChaoxingSubscriptions(ctx context.Context) ([]ChaoxingSubscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE source_type = 'chaoxing' AND enabled = true
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []ChaoxingSubscription
	for rows.Next() {
		sub, err := scanChaoxingSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		return []ChaoxingSubscription{}, nil
	}
	return subs, rows.Err()
}

func (s *Store) AddEmailSubscription(ctx context.Context, userID string, input EmailSubscriptionInput) (string, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return "", err
	}
	input = normalizeEmailInput(input, nil)
	if err := validateEmailInput(input); err != nil {
		return "", err
	}
	if existingID, err := s.findEmailSubscriptionID(ctx, userID, input.Account, input.Mailbox); err != nil {
		return "", err
	} else if existingID != "" {
		enabled := true
		input.Enabled = &enabled
		return existingID, s.UpdateEmailSubscription(ctx, userID, existingID, input)
	}

	id := fmt.Sprintf("email-%d", time.Now().UnixMilli())
	config, err := marshalEmailConfig(input)
	if err != nil {
		return "", err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO subscriptions (id, user_id, source_type, config, enabled)
		VALUES ($1, $2, 'email', $3::jsonb, $4)`,
		id, userID, config, enabled)
	return id, err
}

func (s *Store) UpdateEmailSubscription(ctx context.Context, userID, id string, input EmailSubscriptionInput) error {
	existing, err := s.GetEmailSubscription(ctx, userID, id)
	if err != nil {
		return err
	}
	input = normalizeEmailInput(input, existing)
	if err := validateEmailInput(input); err != nil {
		return err
	}
	config, err := marshalEmailConfig(input)
	if err != nil {
		return err
	}
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE subscriptions
		SET config = $3::jsonb, enabled = $4
		WHERE id = $1 AND user_id = $2 AND source_type = 'email'`,
		id, userID, config, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetEmailSubscription(ctx context.Context, userID, id string) (*EmailSubscription, error) {
	sub, err := scanEmailSubscription(s.db.QueryRow(ctx, `
		SELECT id, user_id, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE user_id = $1 AND id = $2 AND source_type = 'email'`, userID, id))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Store) ListActiveEmailSubscriptions(ctx context.Context) ([]EmailSubscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, config, last_fetched_at, enabled, created_at
		FROM subscriptions
		WHERE source_type = 'email' AND enabled = true
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []EmailSubscription
	for rows.Next() {
		sub, err := scanEmailSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if subs == nil {
		return []EmailSubscription{}, nil
	}
	return subs, rows.Err()
}

func (s *Store) DeleteSubscription(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM subscriptions WHERE id = $1 AND user_id = $2 AND source_type IN ('rss', 'chaoxing', 'email')`,
		id, userID)
	return err
}

func (s *Store) RecordSubscriptionFetchFailure(ctx context.Context, subID string, fetchErr error) error {
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

func (s *Store) UpsertSourceItem(ctx context.Context, input SourceItemInput) (SourceItemRow, SourceItemChange, error) {
	input = normalizeSourceItemInput(input)
	if input.UserID == "" || input.SubscriptionID == "" || input.SourceType == "" || input.ItemType == "" || input.ExternalID == "" {
		return SourceItemRow{}, SourceItemChange{}, fmt.Errorf("source item missing required fields")
	}

	existing, lookupErr := s.getSourceItemByExternal(ctx, input.SubscriptionID, input.ItemType, input.ExternalID)
	if lookupErr != nil && !isNoRows(lookupErr) {
		return SourceItemRow{}, SourceItemChange{}, lookupErr
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return SourceItemRow{}, SourceItemChange{}, fmt.Errorf("source item payload: %w", err)
	}
	created := isNoRows(lookupErr)
	dueChanged := !created && !sameOptionalTime(existing.DueAt, input.DueAt)
	statusChanged := !created && existing.Status != input.Status
	id := existing.ID
	if created {
		id = stableSourceItemID(input.SubscriptionID, input.ItemType, input.ExternalID)
	}
	row, err := scanSourceItem(s.db.QueryRow(ctx, `
		INSERT INTO source_items (
			id, user_id, subscription_id, source_type, item_type, external_id,
			course, title, status, url, due_at, payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb)
		ON CONFLICT (subscription_id, item_type, external_id) DO UPDATE
		SET course = $7,
		    title = $8,
		    status = $9,
		    url = $10,
		    due_at = $11,
		    payload = $12::jsonb,
		    last_seen_at = NOW(),
		    due_notified_at = CASE WHEN $13 THEN NULL ELSE source_items.due_notified_at END,
		    updated_at = NOW()
		RETURNING id, user_id, subscription_id, source_type, item_type, external_id,
		          course, title, status, url, due_at, payload,
		          first_seen_at, last_seen_at, new_notified_at, due_notified_at,
		          created_at, updated_at`,
		id, input.UserID, input.SubscriptionID, input.SourceType, input.ItemType, input.ExternalID,
		input.Course, input.Title, input.Status, input.URL, input.DueAt, payload, dueChanged))
	if err != nil {
		return SourceItemRow{}, SourceItemChange{}, fmt.Errorf("upsert source item: %w", err)
	}
	return row, SourceItemChange{Created: created, StatusChanged: statusChanged, DueChanged: dueChanged}, nil
}

func (s *Store) ListSourceItems(ctx context.Context, userID, sourceType string, limit int) ([]SourceItemRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, subscription_id, source_type, item_type, external_id,
		       course, title, status, url, due_at, payload,
		       first_seen_at, last_seen_at, new_notified_at, due_notified_at,
		       created_at, updated_at
		FROM source_items
		WHERE user_id = $1 AND ($2 = '' OR source_type = $2)
		ORDER BY COALESCE(due_at, last_seen_at) ASC, last_seen_at DESC
		LIMIT $3`, userID, sourceType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SourceItemRow
	for rows.Next() {
		item, err := scanSourceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		return []SourceItemRow{}, nil
	}
	return items, rows.Err()
}

func (s *Store) ListCurrentSourceItems(ctx context.Context, userID, sourceType string, limit int) ([]SourceItemRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := s.db.Query(ctx, `
		SELECT si.id, si.user_id, si.subscription_id, si.source_type, si.item_type, si.external_id,
		       si.course, si.title, si.status, si.url, si.due_at, si.payload,
		       si.first_seen_at, si.last_seen_at, si.new_notified_at, si.due_notified_at,
		       si.created_at, si.updated_at
		FROM source_items si
		JOIN subscriptions sub ON sub.id = si.subscription_id
		WHERE si.user_id = $1
		  AND ($2 = '' OR si.source_type = $2)
		  AND (
		    sub.last_fetched_at IS NULL
		    OR si.last_seen_at >= sub.last_fetched_at - INTERVAL '2 minutes'
		  )
		ORDER BY COALESCE(si.due_at, si.last_seen_at) ASC, si.last_seen_at DESC
		LIMIT $3`, userID, sourceType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SourceItemRow
	for rows.Next() {
		item, err := scanSourceItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		return []SourceItemRow{}, nil
	}
	return items, rows.Err()
}

func (s *Store) MarkSourceItemNewNotified(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE source_items
		SET new_notified_at = COALESCE(new_notified_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1`, id)
	return err
}

func (s *Store) MarkSourceItemDueNotified(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE source_items
		SET due_notified_at = COALESCE(due_notified_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateSourceItemAnalysis(ctx context.Context, id, status, summary, category string, tags []string, articleID string) error {
	payload, err := json.Marshal(map[string]any{
		"summary":    truncateText(summary, 1200),
		"category":   truncateText(category, 80),
		"tags":       tags,
		"article_id": truncateText(articleID, 180),
	})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		UPDATE source_items
		SET status = $2,
		    payload = payload || $3::jsonb,
		    updated_at = NOW()
		WHERE id = $1`,
		id, truncateText(status, 80), payload)
	return err
}

func (s *Store) getSourceItemByExternal(ctx context.Context, subscriptionID, itemType, externalID string) (SourceItemRow, error) {
	return scanSourceItem(s.db.QueryRow(ctx, `
		SELECT id, user_id, subscription_id, source_type, item_type, external_id,
		       course, title, status, url, due_at, payload,
		       first_seen_at, last_seen_at, new_notified_at, due_notified_at,
		       created_at, updated_at
		FROM source_items
		WHERE subscription_id = $1 AND item_type = $2 AND external_id = $3`,
		subscriptionID, itemType, externalID))
}

func scanSourceSubscription(row scanner) (SourceSubscriptionRow, error) {
	var sub SourceSubscriptionRow
	var configRaw []byte
	var lastFetched pgtype.Timestamptz
	err := row.Scan(&sub.ID, &sub.UserID, &sub.SourceType, &configRaw, &lastFetched, &sub.Enabled, &sub.CreatedAt)
	if err != nil {
		return sub, err
	}
	config := decodeConfig(configRaw)
	sub.FeedURL = configString(config, "feed_url")
	sub.Title = configString(config, "title")
	sub.Category = configString(config, "category")
	sub.Account = configString(config, "account")
	sub.Provider = configString(config, "provider")
	sub.Host = configString(config, "host")
	sub.Port = configInt(config, "port", 0)
	sub.Mailbox = configString(config, "mailbox")
	sub.PasswordConfigured = configString(config, "password") != ""
	sub.CookieConfigured = configString(config, "cookie") != ""
	sub.AlertHours = configInt(config, "alert_hours", 24)
	sub.NotifyNew = configBool(config, "notify_new", true)
	sub.NotifyDue = configBool(config, "notify_due", true)
	sub.NotifyImportant = configBool(config, "notify_important", true)
	sub.SyncUnreadOnly = configBool(config, "sync_unread_only", false)
	sub.SinceDays = configInt(config, "since_days", 1)
	sub.MaxMessages = configInt(config, "max_messages", 20)
	sub.LastError = configString(config, "last_error")
	sub.LastErrorAt = parseOptionalTime(configString(config, "last_error_at"))
	if lastFetched.Valid {
		t := lastFetched.Time
		sub.LastFetchedAt = &t
	}
	return sub, nil
}

func scanChaoxingSubscription(row scanner) (ChaoxingSubscription, error) {
	var sub ChaoxingSubscription
	var configRaw []byte
	var lastFetched pgtype.Timestamptz
	err := row.Scan(&sub.ID, &sub.UserID, &configRaw, &lastFetched, &sub.Enabled, &sub.CreatedAt)
	if err != nil {
		return sub, err
	}
	config := decodeConfig(configRaw)
	sub.Title = configString(config, "title")
	sub.Category = configString(config, "category")
	sub.Account = configString(config, "account")
	sub.Password = configString(config, "password")
	sub.Cookie = configString(config, "cookie")
	sub.AlertHours = configInt(config, "alert_hours", 24)
	sub.NotifyNew = configBool(config, "notify_new", true)
	sub.NotifyDue = configBool(config, "notify_due", true)
	sub.LastError = configString(config, "last_error")
	sub.LastErrorAt = parseOptionalTime(configString(config, "last_error_at"))
	if lastFetched.Valid {
		t := lastFetched.Time
		sub.LastFetchedAt = &t
	}
	return sub, nil
}

func scanEmailSubscription(row scanner) (EmailSubscription, error) {
	var sub EmailSubscription
	var configRaw []byte
	var lastFetched pgtype.Timestamptz
	err := row.Scan(&sub.ID, &sub.UserID, &configRaw, &lastFetched, &sub.Enabled, &sub.CreatedAt)
	if err != nil {
		return sub, err
	}
	config := decodeConfig(configRaw)
	sub.Title = configString(config, "title")
	sub.Category = configString(config, "category")
	sub.Account = configString(config, "account")
	sub.Password = configString(config, "password")
	sub.Provider = configString(config, "provider")
	sub.Host = configString(config, "host")
	sub.Port = configInt(config, "port", 993)
	sub.Mailbox = configString(config, "mailbox")
	if sub.Mailbox == "" {
		sub.Mailbox = "INBOX"
	}
	sub.SinceDays = configInt(config, "since_days", 1)
	sub.MaxMessages = configInt(config, "max_messages", 20)
	sub.NotifyImportant = configBool(config, "notify_important", true)
	sub.SyncUnreadOnly = configBool(config, "sync_unread_only", false)
	sub.LastError = configString(config, "last_error")
	sub.LastErrorAt = parseOptionalTime(configString(config, "last_error_at"))
	if lastFetched.Valid {
		t := lastFetched.Time
		sub.LastFetchedAt = &t
	}
	return sub, nil
}

func scanSourceItem(row scanner) (SourceItemRow, error) {
	var item SourceItemRow
	var dueAt, newNotified, dueNotified pgtype.Timestamptz
	var payloadRaw []byte
	err := row.Scan(
		&item.ID, &item.UserID, &item.SubscriptionID, &item.SourceType, &item.ItemType, &item.ExternalID,
		&item.Course, &item.Title, &item.Status, &item.URL, &dueAt, &payloadRaw,
		&item.FirstSeenAt, &item.LastSeenAt, &newNotified, &dueNotified,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return item, err
	}
	if dueAt.Valid {
		t := dueAt.Time
		item.DueAt = &t
	}
	if newNotified.Valid {
		t := newNotified.Time
		item.NewNotifiedAt = &t
	}
	if dueNotified.Valid {
		t := dueNotified.Time
		item.DueNotifiedAt = &t
	}
	item.Payload = decodeConfig(payloadRaw)
	return item, nil
}

func normalizeChaoxingInput(input ChaoxingSubscriptionInput, existing *ChaoxingSubscription) ChaoxingSubscriptionInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Category = strings.TrimSpace(input.Category)
	input.Account = strings.TrimSpace(input.Account)
	input.Cookie = strings.TrimSpace(input.Cookie)
	if input.AlertHours <= 0 {
		input.AlertHours = 24
	}
	if input.AlertHours > 168 {
		input.AlertHours = 168
	}
	if input.NotifyNew == nil {
		value := true
		if existing != nil {
			value = existing.NotifyNew
		}
		input.NotifyNew = &value
	}
	if input.NotifyDue == nil {
		value := true
		if existing != nil {
			value = existing.NotifyDue
		}
		input.NotifyDue = &value
	}
	if existing != nil {
		if input.Title == "" {
			input.Title = existing.Title
		}
		if input.Category == "" {
			input.Category = existing.Category
		}
		if input.Account == "" {
			input.Account = existing.Account
		}
		if input.Password == "" {
			input.Password = existing.Password
		}
		if input.Cookie == "" {
			input.Cookie = existing.Cookie
		}
	}
	return input
}

func validateChaoxingInput(input ChaoxingSubscriptionInput) error {
	if strings.TrimSpace(input.Account) == "" {
		return fmt.Errorf("chaoxing account is required")
	}
	if input.Password == "" && strings.TrimSpace(input.Cookie) == "" {
		return fmt.Errorf("chaoxing password or cookie is required")
	}
	if input.AlertHours < 1 || input.AlertHours > 168 {
		return fmt.Errorf("chaoxing alert_hours must be between 1 and 168")
	}
	return nil
}

func marshalChaoxingConfig(input ChaoxingSubscriptionInput) ([]byte, error) {
	config := map[string]any{
		"title":       input.Title,
		"category":    input.Category,
		"account":     input.Account,
		"password":    input.Password,
		"cookie":      input.Cookie,
		"alert_hours": input.AlertHours,
		"notify_new":  input.NotifyNew != nil && *input.NotifyNew,
		"notify_due":  input.NotifyDue != nil && *input.NotifyDue,
	}
	return json.Marshal(config)
}

func normalizeEmailInput(input EmailSubscriptionInput, existing *EmailSubscription) EmailSubscriptionInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Category = strings.TrimSpace(input.Category)
	input.Account = strings.TrimSpace(input.Account)
	input.Provider = strings.TrimSpace(strings.ToLower(input.Provider))
	input.Host = strings.TrimSpace(strings.ToLower(input.Host))
	input.Mailbox = strings.TrimSpace(input.Mailbox)
	if input.Mailbox == "" {
		input.Mailbox = "INBOX"
	}
	if input.Provider == "" || input.Host == "" || input.Port <= 0 {
		provider, host, port := inferEmailProvider(input.Account)
		if input.Provider == "" {
			input.Provider = provider
		}
		if input.Host == "" {
			input.Host = host
		}
		if input.Port <= 0 {
			input.Port = port
		}
	}
	if input.Port <= 0 {
		input.Port = 993
	}
	if input.SinceDays <= 0 {
		input.SinceDays = 1
	}
	if input.SinceDays > 30 {
		input.SinceDays = 30
	}
	if input.MaxMessages <= 0 {
		input.MaxMessages = 20
	}
	if input.MaxMessages > 100 {
		input.MaxMessages = 100
	}
	if input.NotifyImportant == nil {
		value := true
		if existing != nil {
			value = existing.NotifyImportant
		}
		input.NotifyImportant = &value
	}
	if input.SyncUnreadOnly == nil {
		value := false
		if existing != nil {
			value = existing.SyncUnreadOnly
		}
		input.SyncUnreadOnly = &value
	}
	if existing != nil {
		if input.Title == "" {
			input.Title = existing.Title
		}
		if input.Category == "" {
			input.Category = existing.Category
		}
		if input.Account == "" {
			input.Account = existing.Account
		}
		if input.Password == "" {
			input.Password = existing.Password
		}
		if input.Provider == "" {
			input.Provider = existing.Provider
		}
		if input.Host == "" {
			input.Host = existing.Host
		}
		if input.Port <= 0 {
			input.Port = existing.Port
		}
		if input.Mailbox == "" {
			input.Mailbox = existing.Mailbox
		}
	}
	return input
}

func validateEmailInput(input EmailSubscriptionInput) error {
	if !IsEmailAddress(input.Account) {
		return fmt.Errorf("email account must be a valid email address")
	}
	if input.Password == "" {
		return fmt.Errorf("email app password or authorization code is required")
	}
	if strings.TrimSpace(input.Host) == "" {
		return fmt.Errorf("email imap host is required")
	}
	if input.Port < 1 || input.Port > 65535 {
		return fmt.Errorf("email imap port must be between 1 and 65535")
	}
	if strings.TrimSpace(input.Mailbox) == "" {
		return fmt.Errorf("email mailbox is required")
	}
	return nil
}

func marshalEmailConfig(input EmailSubscriptionInput) ([]byte, error) {
	config := map[string]any{
		"title":            input.Title,
		"category":         input.Category,
		"account":          input.Account,
		"password":         input.Password,
		"provider":         input.Provider,
		"host":             input.Host,
		"port":             input.Port,
		"mailbox":          input.Mailbox,
		"since_days":       input.SinceDays,
		"max_messages":     input.MaxMessages,
		"notify_important": input.NotifyImportant != nil && *input.NotifyImportant,
		"sync_unread_only": input.SyncUnreadOnly != nil && *input.SyncUnreadOnly,
	}
	return json.Marshal(config)
}

func inferEmailProvider(account string) (provider, host string, port int) {
	port = 993
	domain := ""
	if parts := strings.Split(strings.ToLower(strings.TrimSpace(account)), "@"); len(parts) == 2 {
		domain = parts[1]
	}
	switch domain {
	case "gmail.com", "googlemail.com":
		return "gmail", "imap.gmail.com", port
	case "outlook.com", "hotmail.com", "live.com", "msn.com":
		return "outlook", "outlook.office365.com", port
	case "163.com":
		return "163", "imap.163.com", port
	case "126.com":
		return "126", "imap.126.com", port
	case "qq.com", "foxmail.com":
		return "qq", "imap.qq.com", port
	default:
		if domain != "" {
			return "imap", "imap." + domain, port
		}
		return "imap", "", port
	}
}

func (s *Store) findChaoxingSubscriptionID(ctx context.Context, userID, account string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM subscriptions
		WHERE user_id = $1 AND source_type = 'chaoxing' AND config->>'account' = $2
		LIMIT 1`, userID, strings.TrimSpace(account)).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

func (s *Store) findEmailSubscriptionID(ctx context.Context, userID, account, mailbox string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM subscriptions
		WHERE user_id = $1
		  AND source_type = 'email'
		  AND config->>'account' = $2
		  AND COALESCE(config->>'mailbox', 'INBOX') = $3
		LIMIT 1`, userID, strings.TrimSpace(account), strings.TrimSpace(mailbox)).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

func normalizeSourceItemInput(input SourceItemInput) SourceItemInput {
	input.UserID = strings.TrimSpace(input.UserID)
	input.SubscriptionID = strings.TrimSpace(input.SubscriptionID)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.ItemType = strings.TrimSpace(input.ItemType)
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	input.Course = truncateText(strings.TrimSpace(input.Course), 160)
	input.Title = truncateText(strings.TrimSpace(input.Title), 220)
	input.Status = truncateText(strings.TrimSpace(input.Status), 80)
	input.URL = strings.TrimSpace(input.URL)
	if input.ExternalID == "" {
		input.ExternalID = shortHash(strings.Join([]string{input.SourceType, input.ItemType, input.Course, input.Title, input.URL}, "|"))
	}
	if input.Payload == nil {
		input.Payload = map[string]any{}
	}
	return input
}

func decodeConfig(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil || config == nil {
		return map[string]any{}
	}
	return config
}

func configString(config map[string]any, key string) string {
	if value, ok := config[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func configBool(config map[string]any, key string, fallback bool) bool {
	if value, ok := config[key].(bool); ok {
		return value
	}
	return fallback
}

func configInt(config map[string]any, key string, fallback int) int {
	switch value := config[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func parseOptionalTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999999-07", value); err == nil {
		return &t
	}
	return nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || (err != nil && err.Error() == "no rows in result set")
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.UTC().Truncate(time.Second).Equal(b.UTC().Truncate(time.Second))
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func stableSourceItemID(subscriptionID, itemType, externalID string) string {
	return "source-item-" + shortHash(strings.Join([]string{subscriptionID, itemType, externalID}, "|"))
}
