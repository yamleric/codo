package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
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
		                   status, filter_decision, category, tags, summary, error, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			raw_content      = EXCLUDED.raw_content,
			status          = EXCLUDED.status,
			filter_decision = EXCLUDED.filter_decision,
			category        = EXCLUDED.category,
			tags            = EXCLUDED.tags,
			summary         = EXCLUDED.summary,
			error           = EXCLUDED.error,
			updated_at      = EXCLUDED.updated_at`,
		t.ID, t.UserID, string(t.Source), string(t.ContentType),
		t.URL, t.RawContent(),
		string(t.Status()), string(t.FilterDecision()),
		t.Category(), t.Tags(), t.Summary(), t.Error(),
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
	metadata := t.Metadata()
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal article metadata: %w", err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var articleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO articles (id, user_id, task_id, url, url_hash, source, content_type, content, summary, category, tags, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,NOW())
		ON CONFLICT (user_id, url_hash) WHERE url_hash IS NOT NULL DO UPDATE SET
			summary      = EXCLUDED.summary,
			content      = EXCLUDED.content,
			content_type = EXCLUDED.content_type,
			category     = EXCLUDED.category,
			tags         = EXCLUDED.tags,
			metadata     = EXCLUDED.metadata
		RETURNING id`,
		t.ID, t.UserID, t.ID, t.URL, hash,
		string(t.Source), string(t.ContentType), t.RawContent(), t.Summary(), t.Category(), t.Tags(), string(metadataJSON),
	).Scan(&articleID)
	if err != nil {
		return err
	}
	chunkSummary, chunkContent := articleChunkText(t.Summary(), t.RawContent(), metadata)
	if err := replaceArticleChunks(ctx, tx, t.UserID, articleID, chunkSummary, chunkContent); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	Category       string    `json:"category"`
	Tags           []string  `json:"tags"`
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

// ── Knowledge base ───────────────────────────────────────────────────────────

type ArticleRow struct {
	ID          string         `json:"id"`
	UserID      string         `json:"user_id"`
	TaskID      string         `json:"task_id"`
	URL         string         `json:"url"`
	Title       string         `json:"title"`
	Source      string         `json:"source"`
	ContentType string         `json:"content_type"`
	Content     string         `json:"content,omitempty"`
	Summary     string         `json:"summary"`
	Category    string         `json:"category"`
	Tags        []string       `json:"tags"`
	Metadata    map[string]any `json:"metadata"`
	PublishedAt *time.Time     `json:"published_at"`
	CreatedAt   time.Time      `json:"created_at"`
}

type ArticleQuery struct {
	Category string
	Tag      string
	Query    string
	Limit    int
}

type KnowledgeFacets struct {
	Total      int        `json:"total"`
	Categories []FacetRow `json:"categories"`
	Tags       []FacetRow `json:"tags"`
	Sources    []FacetRow `json:"sources"`
}

type FacetRow struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Store) ListArticles(ctx context.Context, userID string, query ArticleQuery) ([]ArticleRow, error) {
	query = normalizeArticleQuery(query)
	rows, err := s.db.Query(ctx, `
		SELECT id,
		       user_id,
		       COALESCE(task_id, ''),
		       COALESCE(url, ''),
		       COALESCE(title, ''),
		       source,
		       COALESCE(content_type, ''),
		       summary,
		       COALESCE(category, ''),
		       COALESCE(tags, '{}'::text[]),
		       COALESCE(metadata, '{}'::jsonb)::text,
		       published_at,
		       created_at
		FROM articles
		WHERE user_id = $1
		  AND ($2 = '' OR category = $2 OR ($2 = '未分类' AND category = ''))
		  AND ($3 = '' OR $3 = ANY(tags))
		  AND (
		    $4 = ''
		    OR url ILIKE '%' || $4 || '%'
		    OR title ILIKE '%' || $4 || '%'
		    OR summary ILIKE '%' || $4 || '%'
		    OR category ILIKE '%' || $4 || '%'
		    OR EXISTS (
		      SELECT 1 FROM unnest(tags) AS tag(value)
		      WHERE tag.value ILIKE '%' || $4 || '%'
		    )
		  )
		ORDER BY COALESCE(published_at, created_at) DESC, created_at DESC
		LIMIT $5`,
		userID, query.Category, query.Tag, query.Query, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []ArticleRow
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if articles == nil {
		return []ArticleRow{}, nil
	}
	return articles, nil
}

func (s *Store) GetArticle(ctx context.Context, userID, articleID string) (ArticleRow, error) {
	var article ArticleRow
	var rawMetadata string
	var publishedAt pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		SELECT id,
		       user_id,
		       COALESCE(task_id, ''),
		       COALESCE(url, ''),
		       COALESCE(title, ''),
		       source,
		       COALESCE(content_type, ''),
		       COALESCE(content, ''),
		       summary,
		       COALESCE(category, ''),
		       COALESCE(tags, '{}'::text[]),
		       COALESCE(metadata, '{}'::jsonb)::text,
		       published_at,
		       created_at
		FROM articles
		WHERE user_id = $1 AND id = $2`,
		userID, articleID).Scan(
		&article.ID,
		&article.UserID,
		&article.TaskID,
		&article.URL,
		&article.Title,
		&article.Source,
		&article.ContentType,
		&article.Content,
		&article.Summary,
		&article.Category,
		&article.Tags,
		&rawMetadata,
		&publishedAt,
		&article.CreatedAt,
	)
	if err != nil {
		return article, err
	}
	if article.Tags == nil {
		article.Tags = []string{}
	}
	article.Metadata = map[string]any{}
	if strings.TrimSpace(rawMetadata) != "" {
		_ = json.Unmarshal([]byte(rawMetadata), &article.Metadata)
	}
	if publishedAt.Valid {
		t := publishedAt.Time
		article.PublishedAt = &t
	}
	return article, nil
}

func (s *Store) KnowledgeFacets(ctx context.Context, userID string) (KnowledgeFacets, error) {
	var facets KnowledgeFacets
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE user_id = $1`, userID).Scan(&facets.Total); err != nil {
		return facets, err
	}
	categories, err := s.facetRows(ctx, `
		SELECT COALESCE(NULLIF(category, ''), '未分类') AS name, COUNT(*)::int
		FROM articles
		WHERE user_id = $1
		GROUP BY name
		ORDER BY COUNT(*) DESC, name ASC
		LIMIT 80`, userID)
	if err != nil {
		return facets, err
	}
	tags, err := s.facetRows(ctx, `
		SELECT tag.value AS name, COUNT(*)::int
		FROM articles
		CROSS JOIN LATERAL unnest(tags) AS tag(value)
		WHERE user_id = $1 AND tag.value <> ''
		GROUP BY tag.value
		ORDER BY COUNT(*) DESC, tag.value ASC
		LIMIT 80`, userID)
	if err != nil {
		return facets, err
	}
	sources, err := s.facetRows(ctx, `
		SELECT COALESCE(NULLIF(source, ''), 'unknown') AS name, COUNT(*)::int
		FROM articles
		WHERE user_id = $1
		GROUP BY name
		ORDER BY COUNT(*) DESC, name ASC
		LIMIT 20`, userID)
	if err != nil {
		return facets, err
	}
	facets.Categories = categories
	facets.Tags = tags
	facets.Sources = sources
	return facets, nil
}

func normalizeArticleQuery(query ArticleQuery) ArticleQuery {
	query.Category = strings.TrimSpace(query.Category)
	query.Tag = strings.TrimSpace(query.Tag)
	query.Query = strings.TrimSpace(query.Query)
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	return query
}

func (s *Store) facetRows(ctx context.Context, sql string, args ...any) ([]FacetRow, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var facets []FacetRow
	for rows.Next() {
		var facet FacetRow
		if err := rows.Scan(&facet.Name, &facet.Count); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if facets == nil {
		return []FacetRow{}, nil
	}
	return facets, nil
}

func scanArticle(row scanner) (ArticleRow, error) {
	var article ArticleRow
	var rawMetadata string
	var publishedAt pgtype.Timestamptz
	err := row.Scan(
		&article.ID,
		&article.UserID,
		&article.TaskID,
		&article.URL,
		&article.Title,
		&article.Source,
		&article.ContentType,
		&article.Summary,
		&article.Category,
		&article.Tags,
		&rawMetadata,
		&publishedAt,
		&article.CreatedAt,
	)
	if err != nil {
		return article, err
	}
	if article.Tags == nil {
		article.Tags = []string{}
	}
	article.Metadata = map[string]any{}
	if strings.TrimSpace(rawMetadata) != "" {
		_ = json.Unmarshal([]byte(rawMetadata), &article.Metadata)
	}
	if publishedAt.Valid {
		t := publishedAt.Time
		article.PublishedAt = &t
	}
	return article, nil
}

type ArticleChunkInput struct {
	Content       string
	TokenEstimate int
}

type SearchChunkRow struct {
	ChunkID     string
	ArticleID   string
	Title       string
	URL         string
	Source      string
	ContentType string
	Summary     string
	Category    string
	Tags        []string
	Content     string
	Score       float64
	CreatedAt   time.Time
}

type ChunkEmbeddingRow struct {
	ID      string
	UserID  string
	Content string
}

func BuildArticleChunks(summary, content string) []ArticleChunkInput {
	parts := make([]string, 0, 2)
	if summary = strings.TrimSpace(summary); summary != "" {
		parts = append(parts, "摘要:\n"+summary)
	}
	if content = strings.TrimSpace(content); content != "" {
		parts = append(parts, "正文:\n"+content)
	}
	body := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if body == "" {
		return []ArticleChunkInput{}
	}

	const maxRunes = 1800
	const overlapRunes = 160
	const maxChunks = 80

	runes := []rune(body)
	chunks := make([]ArticleChunkInput, 0, minInt(maxChunks, len(runes)/maxRunes+1))
	for start := 0; start < len(runes) && len(chunks) < maxChunks; {
		end := minInt(len(runes), start+maxRunes)
		text := strings.TrimSpace(string(runes[start:end]))
		if text != "" {
			chunks = append(chunks, ArticleChunkInput{
				Content:       text,
				TokenEstimate: estimateTokens(text),
			})
		}
		if end >= len(runes) {
			break
		}
		next := end - overlapRunes
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func articleChunkText(summary, content string, metadata map[string]any) (string, string) {
	translation := translationMetadata(metadata)
	if translation == nil {
		return summary, content
	}
	scope, _ := translation["scope"].(string)
	if scope != "knowledge" && scope != "summary_knowledge" {
		return summary, content
	}
	translated, _ := translation["content"].(string)
	translated = strings.TrimSpace(translated)
	if translated == "" {
		return summary, content
	}
	return summary, translated
}

func translationMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	value, ok := metadata["translation"]
	if !ok {
		return nil
	}
	if item, ok := value.(map[string]any); ok {
		return item
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil
	}
	return item
}

func replaceArticleChunks(ctx context.Context, tx pgx.Tx, userID, articleID, summary, content string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM article_chunks WHERE user_id = $1 AND article_id = $2`, userID, articleID); err != nil {
		return err
	}
	chunks := BuildArticleChunks(summary, content)
	for index, chunk := range chunks {
		id := fmt.Sprintf("%s:chunk:%03d", articleID, index)
		if _, err := tx.Exec(ctx, `
			INSERT INTO article_chunks (id, article_id, user_id, chunk_index, content, token_estimate, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
			id, articleID, userID, index, chunk.Content, chunk.TokenEstimate); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SearchArticleChunksKeyword(ctx context.Context, userID, query string, limit int) ([]SearchChunkRow, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []SearchChunkRow{}, nil
	}
	limit = normalizeSearchLimit(limit)
	rows, err := s.db.Query(ctx, `
		WITH params AS (SELECT $2::text AS q)
		SELECT c.id,
		       a.id,
		       COALESCE(a.title, ''),
		       COALESCE(a.url, ''),
		       a.source,
		       COALESCE(a.content_type, ''),
		       a.summary,
		       COALESCE(a.category, ''),
		       COALESCE(a.tags, '{}'::text[]),
		       c.content,
		       (
		         CASE WHEN lower(c.content) LIKE '%' || p.q || '%' THEN 0.55::double precision ELSE 0 END +
		         CASE WHEN lower(COALESCE(a.title, '')) LIKE '%' || p.q || '%' THEN 0.24::double precision ELSE 0 END +
		         CASE WHEN lower(a.summary) LIKE '%' || p.q || '%' THEN 0.20::double precision ELSE 0 END +
		         CASE WHEN lower(COALESCE(a.category, '')) LIKE '%' || p.q || '%' THEN 0.12::double precision ELSE 0 END +
		         CASE WHEN EXISTS (
		           SELECT 1 FROM unnest(a.tags) AS tag(value)
		           WHERE lower(tag.value) LIKE '%' || p.q || '%'
		         ) THEN 0.12::double precision ELSE 0 END +
		         GREATEST(
		           similarity(lower(c.content), p.q),
		           similarity(lower(COALESCE(a.title, '')), p.q),
		           similarity(lower(a.summary), p.q)
		         )::double precision * 0.35::double precision +
		         LEAST(0.05::double precision, 86400.0::double precision / GREATEST(86400.0::double precision, EXTRACT(EPOCH FROM (NOW() - a.created_at))))
		       )::double precision AS score,
		       a.created_at
		FROM article_chunks c
		JOIN articles a ON a.id = c.article_id AND a.user_id = c.user_id
		CROSS JOIN params p
		WHERE c.user_id = $1
		  AND (
		    lower(c.content) LIKE '%' || p.q || '%'
		    OR lower(COALESCE(a.title, '')) LIKE '%' || p.q || '%'
		    OR lower(a.summary) LIKE '%' || p.q || '%'
		    OR lower(COALESCE(a.category, '')) LIKE '%' || p.q || '%'
		    OR EXISTS (
		      SELECT 1 FROM unnest(a.tags) AS tag(value)
		      WHERE lower(tag.value) LIKE '%' || p.q || '%'
		    )
		    OR similarity(lower(c.content), p.q) > 0.08
		    OR similarity(lower(COALESCE(a.title, '')), p.q) > 0.08
		    OR similarity(lower(a.summary), p.q) > 0.08
		  )
		ORDER BY score DESC, COALESCE(a.published_at, a.created_at) DESC
		LIMIT $3`, userID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchChunkRows(rows)
}

func (s *Store) SearchArticleChunksVector(ctx context.Context, userID string, embedding []float32, limit int) ([]SearchChunkRow, error) {
	vector, err := vectorLiteral(embedding)
	if err != nil {
		return nil, err
	}
	limit = normalizeSearchLimit(limit)
	rows, err := s.db.Query(ctx, `
		SELECT c.id,
		       a.id,
		       COALESCE(a.title, ''),
		       COALESCE(a.url, ''),
		       a.source,
		       COALESCE(a.content_type, ''),
		       a.summary,
		       COALESCE(a.category, ''),
		       COALESCE(a.tags, '{}'::text[]),
		       c.content,
		       (1 - (c.embedding <=> $2::vector))::double precision AS score,
		       a.created_at
		FROM article_chunks c
		JOIN articles a ON a.id = c.article_id AND a.user_id = c.user_id
		WHERE c.user_id = $1 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $2::vector, COALESCE(a.published_at, a.created_at) DESC
		LIMIT $3`, userID, vector, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchChunkRows(rows)
}

func (s *Store) ListChunksNeedingEmbedding(ctx context.Context, userID string, limit int) ([]ChunkEmbeddingRow, error) {
	limit = normalizeSearchLimit(limit)
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, content
		FROM article_chunks
		WHERE user_id = $1 AND embedding IS NULL AND content <> ''
		ORDER BY created_at DESC, chunk_index ASC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []ChunkEmbeddingRow
	for rows.Next() {
		var chunk ChunkEmbeddingRow
		if err := rows.Scan(&chunk.ID, &chunk.UserID, &chunk.Content); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if chunks == nil {
		return []ChunkEmbeddingRow{}, nil
	}
	return chunks, nil
}

func (s *Store) UpdateChunkEmbedding(ctx context.Context, userID, chunkID string, embedding []float32) error {
	vector, err := vectorLiteral(embedding)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		UPDATE article_chunks
		SET embedding = $3::vector,
		    updated_at = NOW()
		WHERE user_id = $1 AND id = $2`, userID, chunkID, vector)
	return err
}

func scanSearchChunkRows(rows pgx.Rows) ([]SearchChunkRow, error) {
	var results []SearchChunkRow
	for rows.Next() {
		var row SearchChunkRow
		if err := rows.Scan(
			&row.ChunkID,
			&row.ArticleID,
			&row.Title,
			&row.URL,
			&row.Source,
			&row.ContentType,
			&row.Summary,
			&row.Category,
			&row.Tags,
			&row.Content,
			&row.Score,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		if row.Tags == nil {
			row.Tags = []string{}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		return []SearchChunkRow{}, nil
	}
	return results, nil
}

func vectorLiteral(values []float32) (string, error) {
	const dimensions = 1536
	if len(values) != dimensions {
		return "", fmt.Errorf("embedding dimension %d does not match vector(%d)", len(values), dimensions)
	}
	var b strings.Builder
	b.Grow(len(values) * 10)
	b.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String(), nil
}

func estimateTokens(text string) int {
	runes := len([]rune(text))
	if runes == 0 {
		return 0
	}
	return maxInt(1, (runes+1)/2)
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Bookmarks ────────────────────────────────────────────────────────────────

type BookmarkRow struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	Folder       string     `json:"folder"`
	Note         string     `json:"note"`
	Status       string     `json:"status"`
	LastTaskID   string     `json:"last_task_id"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastError    string     `json:"last_error"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type BookmarkInput struct {
	URL    string
	Title  string
	Folder string
	Note   string
}

type BookmarkImportResult struct {
	Imported  int           `json:"imported"`
	Updated   int           `json:"updated"`
	Skipped   int           `json:"skipped"`
	Bookmarks []BookmarkRow `json:"bookmarks"`
}

func (s *Store) ListBookmarks(ctx context.Context, userID string) ([]BookmarkRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, url, title, folder, note, status,
		       COALESCE(last_task_id, ''), last_synced_at, last_error, created_at, updated_at
		FROM bookmarks
		WHERE user_id = $1
		ORDER BY
		  CASE status WHEN 'pending' THEN 0 WHEN 'failed' THEN 1 WHEN 'syncing' THEN 2 ELSE 3 END,
		  created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bookmarks []BookmarkRow
	for rows.Next() {
		bookmark, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if bookmarks == nil {
		return []BookmarkRow{}, nil
	}
	return bookmarks, nil
}

func (s *Store) AddBookmarks(ctx context.Context, userID string, inputs []BookmarkInput) (BookmarkImportResult, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return BookmarkImportResult{}, err
	}
	result := BookmarkImportResult{Bookmarks: []BookmarkRow{}}
	for _, input := range inputs {
		bookmark, imported, updated, err := s.upsertBookmark(ctx, userID, input)
		if err != nil {
			return result, err
		}
		if bookmark == nil {
			result.Skipped++
			continue
		}
		if imported {
			result.Imported++
		} else if updated {
			result.Updated++
		} else {
			result.Skipped++
		}
		result.Bookmarks = append(result.Bookmarks, *bookmark)
	}
	return result, nil
}

func (s *Store) UpdateBookmark(ctx context.Context, userID, id string, input BookmarkInput) error {
	_, err := s.db.Exec(ctx, `
		UPDATE bookmarks
		SET title = $3,
		    folder = $4,
		    note = $5,
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Folder), strings.TrimSpace(input.Note))
	return err
}

func (s *Store) DeleteBookmark(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM bookmarks WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) ListBookmarksForSync(ctx context.Context, userID string, ids []string) ([]BookmarkRow, error) {
	if len(ids) > 0 {
		rows, err := s.db.Query(ctx, `
			SELECT id, user_id, url, title, folder, note, status,
			       COALESCE(last_task_id, ''), last_synced_at, last_error, created_at, updated_at
			FROM bookmarks
			WHERE user_id = $1 AND id = ANY($2::text[])
			ORDER BY created_at ASC`, userID, ids)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanBookmarks(rows)
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, url, title, folder, note, status,
		       COALESCE(last_task_id, ''), last_synced_at, last_error, created_at, updated_at
		FROM bookmarks
		WHERE user_id = $1 AND status IN ('pending', 'failed')
		ORDER BY created_at ASC
		LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookmarks(rows)
}

func (s *Store) MarkBookmarkSyncing(ctx context.Context, userID, id, taskID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE bookmarks
		SET status = 'syncing',
		    last_task_id = $3,
		    last_error = '',
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID, taskID)
	return err
}

func (s *Store) MarkBookmarkSynced(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE bookmarks
		SET status = 'synced',
		    last_synced_at = NOW(),
		    last_error = '',
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID)
	return err
}

func (s *Store) MarkBookmarkFailed(ctx context.Context, userID, id string, syncErr error) error {
	msg := ""
	if syncErr != nil {
		msg = syncErr.Error()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE bookmarks
		SET status = 'failed',
		    last_error = $3,
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID, truncateText(msg, 420))
	return err
}

func (s *Store) upsertBookmark(ctx context.Context, userID string, input BookmarkInput) (*BookmarkRow, bool, bool, error) {
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return nil, false, false, nil
	}
	hash := contentHash(url)
	existingID, err := s.findBookmarkID(ctx, userID, hash)
	if err != nil {
		return nil, false, false, err
	}
	if existingID != "" {
		bookmark, err := scanBookmark(s.db.QueryRow(ctx, `
			UPDATE bookmarks
			SET title = CASE WHEN $3 <> '' THEN $3 ELSE title END,
			    folder = CASE WHEN $4 <> '' THEN $4 ELSE folder END,
			    note = CASE WHEN $5 <> '' THEN $5 ELSE note END,
			    status = CASE WHEN status = 'synced' THEN status ELSE 'pending' END,
			    last_error = '',
			    updated_at = NOW()
			WHERE id = $1 AND user_id = $2
			RETURNING id, user_id, url, title, folder, note, status,
			          COALESCE(last_task_id, ''), last_synced_at, last_error, created_at, updated_at`,
			existingID, userID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Folder), strings.TrimSpace(input.Note)))
		if err != nil {
			return nil, false, false, err
		}
		return &bookmark, false, true, nil
	}
	id := fmt.Sprintf("bookmark-%d", time.Now().UnixNano())
	bookmark, err := scanBookmark(s.db.QueryRow(ctx, `
		INSERT INTO bookmarks (id, user_id, url, url_hash, title, folder, note, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		RETURNING id, user_id, url, title, folder, note, status,
		          COALESCE(last_task_id, ''), last_synced_at, last_error, created_at, updated_at`,
		id, userID, url, hash, strings.TrimSpace(input.Title), strings.TrimSpace(input.Folder), strings.TrimSpace(input.Note)))
	if err != nil {
		return nil, false, false, err
	}
	return &bookmark, true, false, nil
}

func (s *Store) findBookmarkID(ctx context.Context, userID, hash string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM bookmarks
		WHERE user_id = $1 AND url_hash = $2
		LIMIT 1`, userID, hash).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

func scanBookmarks(rows pgx.Rows) ([]BookmarkRow, error) {
	var bookmarks []BookmarkRow
	for rows.Next() {
		bookmark, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, bookmark)
	}
	if bookmarks == nil {
		return []BookmarkRow{}, nil
	}
	return bookmarks, nil
}

func scanBookmark(row scanner) (BookmarkRow, error) {
	var bookmark BookmarkRow
	var lastSynced pgtype.Timestamptz
	err := row.Scan(
		&bookmark.ID, &bookmark.UserID, &bookmark.URL, &bookmark.Title, &bookmark.Folder, &bookmark.Note,
		&bookmark.Status, &bookmark.LastTaskID, &lastSynced, &bookmark.LastError, &bookmark.CreatedAt, &bookmark.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return bookmark, err
		}
		return bookmark, err
	}
	if lastSynced.Valid {
		t := lastSynced.Time
		bookmark.LastSyncedAt = &t
	}
	return bookmark, nil
}

func (s *Store) ListTasks(ctx context.Context, userID string, limit int) ([]TaskRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source, content_type, url, status, filter_decision,
		       COALESCE(category, ''), COALESCE(tags, '{}'::text[]),
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
			&t.Status, &t.FilterDecision, &t.Category, &t.Tags, &t.Summary, &t.Error, &createdAt); err != nil {
			return nil, err
		}
		if t.Tags == nil {
			t.Tags = []string{}
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
		       COALESCE(category, ''), COALESCE(tags, '{}'::text[]),
		       summary, error, created_at
		FROM tasks WHERE id = $1`, id).Scan(
		&t.ID, &t.Source, &t.ContentType, &t.URL,
		&t.Status, &t.FilterDecision, &t.Category, &t.Tags, &t.Summary, &t.Error, &createdAt)
	if err != nil {
		return nil, err
	}
	if t.Tags == nil {
		t.Tags = []string{}
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
	Username       string          `json:"username"`
	NotifyChannel  string          `json:"notify_channel"`
	FilterKeywords []string        `json:"filter_keywords"`
	ModelPolicy    UserModelPolicy `json:"model_policy"`
	DailyReport    DailyReport     `json:"daily_report"`
}

type UserModelPolicy struct {
	SummaryStyle    string            `json:"summary_style"`
	Language        string            `json:"language"`
	MaxSummaryChars int               `json:"max_summary_chars"`
	NotifyPolicy    string            `json:"notify_policy"`
	Translation     TranslationPolicy `json:"translation"`
}

type TranslationPolicy struct {
	Enabled        bool   `json:"enabled"`
	Mode           string `json:"mode"`
	TargetLanguage string `json:"target_language"`
	Scope          string `json:"scope"`
	MaxChars       int    `json:"max_chars"`
}

type DailyReport struct {
	Enabled         bool     `json:"enabled"`
	Email           string   `json:"email"`
	Hour            int      `json:"hour"`
	Timezone        string   `json:"timezone"`
	MaxItems        int      `json:"max_items"`
	Frequency       string   `json:"frequency"`
	Channels        []string `json:"channels"`
	Sources         []string `json:"sources"`
	Categories      []string `json:"categories"`
	CategoryMode    string   `json:"category_mode"`
	SplitByCategory bool     `json:"split_by_category"`
}

func DefaultUserModelPolicy() UserModelPolicy {
	return UserModelPolicy{
		SummaryStyle:    "concise",
		Language:        "zh-CN",
		MaxSummaryChars: 300,
		NotifyPolicy:    "pass_only",
		Translation:     DefaultTranslationPolicy(),
	}
}

func DefaultTranslationPolicy() TranslationPolicy {
	return TranslationPolicy{
		Enabled:        false,
		Mode:           "english_only",
		TargetLanguage: "zh-CN",
		Scope:          "summary_knowledge",
		MaxChars:       8000,
	}
}

func DefaultDailyReport() DailyReport {
	return DailyReport{
		Enabled:      false,
		Email:        "",
		Hour:         21,
		Timezone:     "Asia/Shanghai",
		MaxItems:     20,
		Frequency:    "daily",
		Channels:     []string{"email"},
		Sources:      []string{},
		Categories:   []string{},
		CategoryMode: "all",
	}
}

func NormalizeUserSettings(settings UserSettings) UserSettings {
	switch settings.NotifyChannel {
	case "telegram", "email", "none":
	default:
		settings.NotifyChannel = "telegram"
	}
	settings.FilterKeywords = normalizeKeywords(settings.FilterKeywords)
	settings.ModelPolicy = NormalizeUserModelPolicy(settings.ModelPolicy)
	settings.DailyReport = NormalizeDailyReport(settings.DailyReport)
	if settings.DailyReport.Enabled && ReportUsesChannel(settings.DailyReport, "email") && DailyReportRecipient(settings.DailyReport, settings.Username) == "" {
		settings.DailyReport.Channels = removeString(settings.DailyReport.Channels, "email")
		if len(settings.DailyReport.Channels) == 0 {
			settings.DailyReport.Enabled = false
			settings.DailyReport.Channels = []string{"email"}
		}
	}
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
	policy.Translation = NormalizeTranslationPolicy(policy.Translation)
	return policy
}

func NormalizeTranslationPolicy(policy TranslationPolicy) TranslationPolicy {
	defaults := DefaultTranslationPolicy()
	switch strings.TrimSpace(strings.ToLower(policy.Mode)) {
	case "english_only":
		policy.Mode = "english_only"
	default:
		policy.Mode = defaults.Mode
	}
	switch strings.TrimSpace(policy.TargetLanguage) {
	case "zh-CN":
		policy.TargetLanguage = "zh-CN"
	default:
		policy.TargetLanguage = defaults.TargetLanguage
	}
	switch strings.TrimSpace(strings.ToLower(policy.Scope)) {
	case "summary", "knowledge", "summary_knowledge":
		policy.Scope = strings.TrimSpace(strings.ToLower(policy.Scope))
	default:
		policy.Scope = defaults.Scope
	}
	if policy.MaxChars < 1000 {
		policy.MaxChars = defaults.MaxChars
	}
	if policy.MaxChars > 30000 {
		policy.MaxChars = 30000
	}
	return policy
}

func NormalizeDailyReport(report DailyReport) DailyReport {
	defaults := DefaultDailyReport()
	report.Email = strings.TrimSpace(report.Email)
	if len(report.Email) > 160 {
		report.Email = ""
	}
	if report.Email != "" {
		if _, err := mail.ParseAddress(report.Email); err != nil {
			report.Email = ""
		}
	}
	if report.Hour < 0 || report.Hour > 23 {
		report.Hour = defaults.Hour
	}
	report.Timezone = strings.TrimSpace(report.Timezone)
	if report.Timezone == "" {
		report.Timezone = defaults.Timezone
	}
	if _, err := time.LoadLocation(report.Timezone); err != nil {
		report.Timezone = defaults.Timezone
	}
	if report.MaxItems <= 0 {
		report.MaxItems = defaults.MaxItems
	}
	if report.MaxItems > 80 {
		report.MaxItems = 80
	}
	switch strings.TrimSpace(strings.ToLower(report.Frequency)) {
	case "daily", "weekly", "monthly":
		report.Frequency = strings.TrimSpace(strings.ToLower(report.Frequency))
	default:
		report.Frequency = defaults.Frequency
	}
	report.Channels = normalizeStringEnumList(report.Channels, map[string]struct{}{
		"email":    {},
		"telegram": {},
	}, 2)
	if len(report.Channels) == 0 {
		report.Channels = append([]string{}, defaults.Channels...)
	}
	report.Sources = normalizeStringEnumList(report.Sources, map[string]struct{}{
		"manual":     {},
		"bookmark":   {},
		"linux_do":   {},
		"rss":        {},
		"wechat_mp":  {},
		"email":      {},
		"chaoxing":   {},
		"group_chat": {},
	}, 16)
	report.Categories = normalizeReportCategories(report.Categories)
	switch strings.TrimSpace(strings.ToLower(report.CategoryMode)) {
	case "all", "include", "exclude":
		report.CategoryMode = strings.TrimSpace(strings.ToLower(report.CategoryMode))
	default:
		report.CategoryMode = defaults.CategoryMode
	}
	if len(report.Categories) == 0 {
		report.CategoryMode = "all"
	}
	return report
}

func (s *Store) GetUserSettings(ctx context.Context, userID string) (UserSettings, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return UserSettings{}, err
	}

	settings := UserSettings{}
	var rawPolicy string
	err := s.db.QueryRow(ctx, `
		SELECT id,
		       COALESCE(username, ''),
		       COALESCE(NULLIF(notify_channel, ''), 'telegram'),
		       COALESCE(filter_keywords, '{}'::text[]),
		       COALESCE(model_policy, '{}'::jsonb)::text
		FROM users
		WHERE id = $1`, userID).Scan(
		&settings.UserID,
		&settings.Username,
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
	settings.DailyReport = dailyReportFromRawPolicy(rawPolicy)
	return NormalizeUserSettings(settings), nil
}

func IsEmailAddress(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, err := mail.ParseAddress(value)
	return err == nil
}

func DailyReportRecipient(report DailyReport, username string) string {
	if IsEmailAddress(report.Email) {
		return strings.TrimSpace(report.Email)
	}
	username = strings.TrimSpace(username)
	if IsEmailAddress(username) {
		return username
	}
	return ""
}

func ReportUsesChannel(report DailyReport, channel string) bool {
	channel = strings.TrimSpace(strings.ToLower(channel))
	for _, item := range NormalizeDailyReport(report).Channels {
		if item == channel {
			return true
		}
	}
	return false
}

func (s *Store) UpdateUserSettings(ctx context.Context, settings UserSettings) error {
	if err := s.ensureUser(ctx, settings.UserID); err != nil {
		return err
	}
	settings = NormalizeUserSettings(settings)
	policyPayload := map[string]any{
		"summary_style":     settings.ModelPolicy.SummaryStyle,
		"language":          settings.ModelPolicy.Language,
		"max_summary_chars": settings.ModelPolicy.MaxSummaryChars,
		"notify_policy":     settings.ModelPolicy.NotifyPolicy,
		"translation":       settings.ModelPolicy.Translation,
		"daily_report":      settings.DailyReport,
	}
	policyJSON, err := json.Marshal(policyPayload)
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

func dailyReportFromRawPolicy(rawPolicy string) DailyReport {
	defaults := DefaultDailyReport()
	if strings.TrimSpace(rawPolicy) == "" {
		return defaults
	}
	var payload struct {
		DailyReport *struct {
			Enabled         *bool     `json:"enabled"`
			Email           *string   `json:"email"`
			Hour            *int      `json:"hour"`
			Timezone        *string   `json:"timezone"`
			MaxItems        *int      `json:"max_items"`
			Frequency       *string   `json:"frequency"`
			Channels        *[]string `json:"channels"`
			Sources         *[]string `json:"sources"`
			Categories      *[]string `json:"categories"`
			CategoryMode    *string   `json:"category_mode"`
			SplitByCategory *bool     `json:"split_by_category"`
		} `json:"daily_report"`
	}
	if err := json.Unmarshal([]byte(rawPolicy), &payload); err != nil {
		return defaults
	}
	if payload.DailyReport == nil {
		return defaults
	}
	report := defaults
	if payload.DailyReport.Enabled != nil {
		report.Enabled = *payload.DailyReport.Enabled
	}
	if payload.DailyReport.Email != nil {
		report.Email = *payload.DailyReport.Email
	}
	if payload.DailyReport.Hour != nil {
		report.Hour = *payload.DailyReport.Hour
	}
	if payload.DailyReport.Timezone != nil {
		report.Timezone = *payload.DailyReport.Timezone
	}
	if payload.DailyReport.MaxItems != nil {
		report.MaxItems = *payload.DailyReport.MaxItems
	}
	if payload.DailyReport.Frequency != nil {
		report.Frequency = *payload.DailyReport.Frequency
	}
	if payload.DailyReport.Channels != nil {
		report.Channels = *payload.DailyReport.Channels
	}
	if payload.DailyReport.Sources != nil {
		report.Sources = *payload.DailyReport.Sources
	}
	if payload.DailyReport.Categories != nil {
		report.Categories = *payload.DailyReport.Categories
	}
	if payload.DailyReport.CategoryMode != nil {
		report.CategoryMode = *payload.DailyReport.CategoryMode
	}
	if payload.DailyReport.SplitByCategory != nil {
		report.SplitByCategory = *payload.DailyReport.SplitByCategory
	}
	return report
}

func (s *Store) GetLLMPreferences(ctx context.Context, userID string) (llm.UserPreferences, error) {
	settings, err := s.GetUserSettings(ctx, userID)
	if err != nil {
		return llm.UserPreferences{}, err
	}
	memoryEnabled, preferenceMemory, err := s.PreferencePrompt(ctx, userID)
	if err != nil {
		memoryEnabled = false
		preferenceMemory = ""
	}
	return llm.UserPreferences{
		FilterKeywords:  settings.FilterKeywords,
		SummaryStyle:    settings.ModelPolicy.SummaryStyle,
		Language:        settings.ModelPolicy.Language,
		MaxSummaryChars: settings.ModelPolicy.MaxSummaryChars,
		Translation: llm.TranslationPolicy{
			Enabled:        settings.ModelPolicy.Translation.Enabled,
			Mode:           settings.ModelPolicy.Translation.Mode,
			TargetLanguage: settings.ModelPolicy.Translation.TargetLanguage,
			Scope:          settings.ModelPolicy.Translation.Scope,
			MaxChars:       settings.ModelPolicy.Translation.MaxChars,
		},
		MemoryEnabled:    memoryEnabled,
		PreferenceMemory: preferenceMemory,
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

func normalizeStringEnumList(values []string, allowed map[string]struct{}, limit int) []string {
	if limit <= 0 {
		limit = len(values)
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func normalizeReportCategories(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = truncateText(strings.TrimSpace(value), 24)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= 24 {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func removeString(values []string, target string) []string {
	target = strings.TrimSpace(strings.ToLower(target))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(strings.ToLower(value)) == target {
			continue
		}
		out = append(out, value)
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
