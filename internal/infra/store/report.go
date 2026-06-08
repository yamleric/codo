package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type DailyArticleRow struct {
	ID        string
	URL       string
	Source    string
	Summary   string
	Category  string
	Tags      []string
	CreatedAt time.Time
}

type DailyArticleQuery struct {
	Limit        int
	Sources      []string
	Categories   []string
	CategoryMode string
}

func (s *Store) ListDailyArticles(ctx context.Context, userID string, start, end time.Time, query DailyArticleQuery) ([]DailyArticleRow, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.CategoryMode != "include" && query.CategoryMode != "exclude" {
		query.CategoryMode = "all"
	}
	if len(query.Categories) == 0 {
		query.CategoryMode = "all"
	}
	if query.Sources == nil {
		query.Sources = []string{}
	}
	if query.Categories == nil {
		query.Categories = []string{}
	}
	rows, err := s.db.Query(ctx, `
		SELECT id,
		       COALESCE(url, ''),
		       source,
		       summary,
		       COALESCE(category, ''),
		       COALESCE(tags, '{}'::text[]),
		       created_at
		FROM articles
		WHERE user_id = $1
		  AND created_at >= $2
		  AND created_at < $3
		  AND summary <> ''
		  AND (cardinality($4::text[]) = 0 OR source = ANY($4::text[]))
		  AND (
		    $5 = 'all'
		    OR ($5 = 'include' AND COALESCE(NULLIF(category, ''), '未分类') = ANY($6::text[]))
		    OR ($5 = 'exclude' AND NOT (COALESCE(NULLIF(category, ''), '未分类') = ANY($6::text[])))
		  )
		ORDER BY created_at DESC
		LIMIT $7`, userID, start, end, query.Sources, query.CategoryMode, query.Categories, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []DailyArticleRow
	for rows.Next() {
		var article DailyArticleRow
		if err := rows.Scan(
			&article.ID,
			&article.URL,
			&article.Source,
			&article.Summary,
			&article.Category,
			&article.Tags,
			&article.CreatedAt,
		); err != nil {
			return nil, err
		}
		if article.Tags == nil {
			article.Tags = []string{}
		}
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if articles == nil {
		return []DailyArticleRow{}, nil
	}
	return articles, nil
}

func (s *Store) StartDailyReport(ctx context.Context, userID, reportDate string) (bool, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return false, err
	}
	id := fmt.Sprintf("daily-%s-%d", reportDate, time.Now().UnixNano())
	var status string
	err := s.db.QueryRow(ctx, `
		INSERT INTO daily_reports (id, user_id, report_date, status, last_error, created_at, updated_at)
		VALUES ($1, $2, $3::date, 'running', '', NOW(), NOW())
		ON CONFLICT (user_id, report_date) DO UPDATE
		SET status = 'running',
		    last_error = '',
		    updated_at = NOW()
		WHERE daily_reports.status = 'failed'
		   OR (
		     daily_reports.status = 'running'
		     AND daily_reports.updated_at < NOW() - INTERVAL '1 hour'
		   )
		RETURNING status`, id, userID, reportDate).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) MarkDailyReportSent(ctx context.Context, userID, reportDate string, itemCount int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE daily_reports
		SET status = 'sent',
		    item_count = $3,
		    last_error = '',
		    sent_at = NOW(),
		    updated_at = NOW()
		WHERE user_id = $1 AND report_date = $2::date`,
		userID, reportDate, itemCount)
	return err
}

func (s *Store) MarkDailyReportSkipped(ctx context.Context, userID, reportDate string, itemCount int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE daily_reports
		SET status = 'skipped',
		    item_count = $3,
		    last_error = '',
		    updated_at = NOW()
		WHERE user_id = $1 AND report_date = $2::date`,
		userID, reportDate, itemCount)
	return err
}

func (s *Store) MarkDailyReportFailed(ctx context.Context, userID, reportDate string, reportErr error) error {
	msg := ""
	if reportErr != nil {
		msg = reportErr.Error()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE daily_reports
		SET status = 'failed',
		    last_error = $3,
		    updated_at = NOW()
		WHERE user_id = $1 AND report_date = $2::date`,
		userID, reportDate, truncateText(msg, 420))
	return err
}
