package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	feedbackUseful         = "useful"
	feedbackNotUseful      = "not_useful"
	feedbackNotifySimilar  = "notify_similar"
	feedbackSilentSimilar  = "silent_similar"
	feedbackDiscardSimilar = "discard_similar"

	memoryInterest = "interest"
	memoryNotify   = "notify"
	memorySilent   = "silent"
	memoryReject   = "reject"
	memoryIntent   = "intent"
)

type FeedbackInput struct {
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Rating     string `json:"rating"`
	Intent     string `json:"intent"`
	Comment    string `json:"comment"`
	Source     string `json:"source"`
}

type ContentFeedbackRow struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Rating     string    `json:"rating"`
	Intent     string    `json:"intent"`
	Comment    string    `json:"comment"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type UserMemoryInput struct {
	MemoryType string  `json:"memory_type"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	Disabled   bool    `json:"disabled"`
}

type UserMemoryRow struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	MemoryType string     `json:"memory_type"`
	Content    string     `json:"content"`
	Confidence float64    `json:"confidence"`
	SourceType string     `json:"source_type"`
	SourceID   string     `json:"source_id"`
	DisabledAt *time.Time `json:"disabled_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type PreferenceProfile struct {
	UserID             string    `json:"user_id"`
	MemoryEnabled      bool      `json:"memory_enabled"`
	Interests          []string  `json:"interests"`
	NotifyPreferences  []string  `json:"notify_preferences"`
	ArchivePreferences []string  `json:"archive_preferences"`
	RejectPatterns     []string  `json:"reject_patterns"`
	RecentIntents      []string  `json:"recent_intents"`
	FeedbackCount      int       `json:"feedback_count"`
	MemoryCount        int       `json:"memory_count"`
	Version            int       `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type preferenceProfilePayload struct {
	Interests          []string `json:"interests"`
	NotifyPreferences  []string `json:"notify_preferences"`
	ArchivePreferences []string `json:"archive_preferences"`
	RejectPatterns     []string `json:"reject_patterns"`
	RecentIntents      []string `json:"recent_intents"`
	FeedbackCount      int      `json:"feedback_count"`
	MemoryCount        int      `json:"memory_count"`
}

func (s *Store) SaveContentFeedback(ctx context.Context, userID string, input FeedbackInput) (ContentFeedbackRow, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return ContentFeedbackRow{}, err
	}
	input = normalizeFeedbackInput(input)
	if input.TargetType == "" || input.TargetID == "" {
		return ContentFeedbackRow{}, fmt.Errorf("feedback target is required")
	}
	if input.Rating == "" {
		return ContentFeedbackRow{}, fmt.Errorf("feedback rating is required")
	}

	id := fmt.Sprintf("feedback-%d", time.Now().UnixNano())
	row, err := scanFeedback(s.db.QueryRow(ctx, `
		INSERT INTO content_feedback (id, user_id, target_type, target_id, rating, intent, comment, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (user_id, target_type, target_id) DO UPDATE SET
			rating = EXCLUDED.rating,
			intent = EXCLUDED.intent,
			comment = EXCLUDED.comment,
			source = EXCLUDED.source,
			updated_at = NOW()
		RETURNING id, user_id, target_type, target_id, rating, intent, comment, source, created_at, updated_at`,
		id, userID, input.TargetType, input.TargetID, input.Rating, input.Intent, input.Comment, input.Source))
	if err != nil {
		return ContentFeedbackRow{}, err
	}

	if err := s.upsertMemoryFromFeedback(ctx, row); err != nil {
		return ContentFeedbackRow{}, err
	}
	if _, err := s.RebuildPreferenceProfile(ctx, userID); err != nil {
		return ContentFeedbackRow{}, err
	}
	return row, nil
}

func (s *Store) AddManualIntentMemory(ctx context.Context, userID, sourceID, url, intent string) (UserMemoryRow, error) {
	input := UserMemoryInput{
		MemoryType: memoryIntent,
		Content:    "用户提交链接时的意图：" + strings.TrimSpace(intent) + "\n链接：" + strings.TrimSpace(url),
		Confidence: 0.9,
		SourceType: "manual_intent",
		SourceID:   sourceID,
	}
	memory, err := s.UpsertUserMemory(ctx, userID, input)
	if err != nil {
		return UserMemoryRow{}, err
	}
	_, err = s.RebuildPreferenceProfile(ctx, userID)
	return memory, err
}

func (s *Store) UpsertUserMemory(ctx context.Context, userID string, input UserMemoryInput) (UserMemoryRow, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return UserMemoryRow{}, err
	}
	input = normalizeMemoryInput(input)
	if input.Content == "" {
		return UserMemoryRow{}, fmt.Errorf("memory content is required")
	}

	id := fmt.Sprintf("memory-%d", time.Now().UnixNano())
	var row UserMemoryRow
	var err error
	if input.SourceType != "" && input.SourceID != "" {
		row, err = scanMemory(s.db.QueryRow(ctx, `
			INSERT INTO user_memories (id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $8 THEN NOW() ELSE NULL END, NOW(), NOW())
			ON CONFLICT (user_id, source_type, source_id) WHERE source_type <> '' AND source_id <> '' DO UPDATE SET
				memory_type = EXCLUDED.memory_type,
				content = EXCLUDED.content,
				confidence = EXCLUDED.confidence,
				disabled_at = EXCLUDED.disabled_at,
				updated_at = NOW()
			RETURNING id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at`,
			id, userID, input.MemoryType, input.Content, input.Confidence, input.SourceType, input.SourceID, input.Disabled))
	} else {
		row, err = scanMemory(s.db.QueryRow(ctx, `
			INSERT INTO user_memories (id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, '', '', CASE WHEN $6 THEN NOW() ELSE NULL END, NOW(), NOW())
			RETURNING id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at`,
			id, userID, input.MemoryType, input.Content, input.Confidence, input.Disabled))
	}
	if err != nil {
		return UserMemoryRow{}, err
	}
	if _, err := s.RebuildPreferenceProfile(ctx, userID); err != nil {
		return UserMemoryRow{}, err
	}
	return row, nil
}

func (s *Store) UpdateUserMemory(ctx context.Context, userID, id string, input UserMemoryInput) (UserMemoryRow, error) {
	input = normalizeMemoryInput(input)
	if input.Content == "" {
		return UserMemoryRow{}, fmt.Errorf("memory content is required")
	}
	row, err := scanMemory(s.db.QueryRow(ctx, `
		UPDATE user_memories
		SET memory_type = $3,
		    content = $4,
		    confidence = $5,
		    disabled_at = CASE WHEN $6 THEN COALESCE(disabled_at, NOW()) ELSE NULL END,
		    updated_at = NOW()
		WHERE user_id = $1 AND id = $2
		RETURNING id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at`,
		userID, id, input.MemoryType, input.Content, input.Confidence, input.Disabled))
	if err != nil {
		return UserMemoryRow{}, err
	}
	if _, err := s.RebuildPreferenceProfile(ctx, userID); err != nil {
		return UserMemoryRow{}, err
	}
	return row, nil
}

func (s *Store) DeleteUserMemory(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM user_memories WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = s.RebuildPreferenceProfile(ctx, userID)
	return err
}

func (s *Store) ListUserMemories(ctx context.Context, userID string, includeDisabled bool, limit int) ([]UserMemoryRow, error) {
	limit = normalizeMemoryLimit(limit)
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, memory_type, content, confidence, source_type, source_id, disabled_at, created_at, updated_at
		FROM user_memories
		WHERE user_id = $1 AND ($2::boolean OR disabled_at IS NULL)
		ORDER BY disabled_at NULLS FIRST, updated_at DESC
		LIMIT $3`, userID, includeDisabled, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryRows(rows)
}

func (s *Store) ListContentFeedback(ctx context.Context, userID string, limit int) ([]ContentFeedbackRow, error) {
	limit = normalizeMemoryLimit(limit)
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, target_type, target_id, rating, intent, comment, source, created_at, updated_at
		FROM content_feedback
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var feedback []ContentFeedbackRow
	for rows.Next() {
		row, err := scanFeedback(rows)
		if err != nil {
			return nil, err
		}
		feedback = append(feedback, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if feedback == nil {
		return []ContentFeedbackRow{}, nil
	}
	return feedback, nil
}

func (s *Store) GetPreferenceProfile(ctx context.Context, userID string) (PreferenceProfile, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return PreferenceProfile{}, err
	}
	if err := s.ensurePreferenceProfile(ctx, userID); err != nil {
		return PreferenceProfile{}, err
	}
	var raw string
	var profile PreferenceProfile
	profile.UserID = userID
	err := s.db.QueryRow(ctx, `
		SELECT memory_enabled, COALESCE(profile_json, '{}'::jsonb)::text, version, updated_at
		FROM preference_profiles
		WHERE user_id = $1`, userID).Scan(&profile.MemoryEnabled, &raw, &profile.Version, &profile.UpdatedAt)
	if err != nil {
		return PreferenceProfile{}, err
	}
	payload := preferenceProfilePayload{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &payload)
	}
	profile.Interests = normalizeMemoryList(payload.Interests, 12)
	profile.NotifyPreferences = normalizeMemoryList(payload.NotifyPreferences, 12)
	profile.ArchivePreferences = normalizeMemoryList(payload.ArchivePreferences, 12)
	profile.RejectPatterns = normalizeMemoryList(payload.RejectPatterns, 12)
	profile.RecentIntents = normalizeMemoryList(payload.RecentIntents, 12)
	profile.FeedbackCount = payload.FeedbackCount
	profile.MemoryCount = payload.MemoryCount
	return profile, nil
}

func (s *Store) SetPreferenceMemoryEnabled(ctx context.Context, userID string, enabled bool) (PreferenceProfile, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return PreferenceProfile{}, err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO preference_profiles (user_id, memory_enabled, profile_json, version, created_at, updated_at)
		VALUES ($1, $2, '{}'::jsonb, 1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			memory_enabled = EXCLUDED.memory_enabled,
			updated_at = NOW()`, userID, enabled)
	if err != nil {
		return PreferenceProfile{}, err
	}
	return s.GetPreferenceProfile(ctx, userID)
}

func (s *Store) RebuildPreferenceProfile(ctx context.Context, userID string) (PreferenceProfile, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return PreferenceProfile{}, err
	}
	existing, err := s.GetPreferenceProfile(ctx, userID)
	if err != nil {
		return PreferenceProfile{}, err
	}
	memories, err := s.ListUserMemories(ctx, userID, false, 80)
	if err != nil {
		return PreferenceProfile{}, err
	}
	payload := preferenceProfilePayload{}
	for _, memory := range memories {
		brief := memoryBrief(memory.Content)
		if brief == "" {
			continue
		}
		switch memory.MemoryType {
		case memoryNotify:
			payload.NotifyPreferences = appendUniqueLimited(payload.NotifyPreferences, brief, 10)
		case memorySilent:
			payload.ArchivePreferences = appendUniqueLimited(payload.ArchivePreferences, brief, 10)
		case memoryReject:
			payload.RejectPatterns = appendUniqueLimited(payload.RejectPatterns, brief, 10)
		case memoryIntent:
			payload.RecentIntents = appendUniqueLimited(payload.RecentIntents, brief, 10)
		default:
			payload.Interests = appendUniqueLimited(payload.Interests, brief, 10)
		}
	}
	payload.MemoryCount = len(memories)
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*)::int FROM content_feedback WHERE user_id = $1`, userID).Scan(&payload.FeedbackCount); err != nil {
		return PreferenceProfile{}, err
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return PreferenceProfile{}, err
	}
	var profile PreferenceProfile
	var rawProfile string
	profile.UserID = userID
	err = s.db.QueryRow(ctx, `
		INSERT INTO preference_profiles (user_id, memory_enabled, profile_json, version, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, 1, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			profile_json = EXCLUDED.profile_json,
			version = preference_profiles.version + 1,
			updated_at = NOW()
		RETURNING memory_enabled, COALESCE(profile_json, '{}'::jsonb)::text, version, updated_at`,
		userID, existing.MemoryEnabled, string(rawPayload)).Scan(&profile.MemoryEnabled, &rawProfile, &profile.Version, &profile.UpdatedAt)
	if err != nil {
		return PreferenceProfile{}, err
	}
	out, err := profileFromRaw(userID, profile.MemoryEnabled, rawProfile, profile.Version, profile.UpdatedAt)
	if err != nil {
		return PreferenceProfile{}, err
	}
	return out, nil
}

func (s *Store) PreferencePrompt(ctx context.Context, userID string) (bool, string, error) {
	profile, err := s.GetPreferenceProfile(ctx, userID)
	if err != nil {
		return false, "", err
	}
	if !profile.MemoryEnabled {
		return false, "", nil
	}
	memories, err := s.ListUserMemories(ctx, userID, false, 16)
	if err != nil {
		return false, "", err
	}
	var b strings.Builder
	writePromptList(&b, "近期收藏意图", profile.RecentIntents, 5)
	writePromptList(&b, "感兴趣内容", profile.Interests, 5)
	writePromptList(&b, "优先通知", profile.NotifyPreferences, 5)
	writePromptList(&b, "静默归档", profile.ArchivePreferences, 5)
	writePromptList(&b, "降低优先级或丢弃", profile.RejectPatterns, 5)
	if len(memories) > 0 {
		b.WriteString("近期具体反馈：\n")
		for i, memory := range memories {
			if i >= 8 {
				break
			}
			brief := memoryBrief(memory.Content)
			if brief == "" {
				continue
			}
			fmt.Fprintf(&b, "- [%s] %s\n", memory.MemoryType, brief)
		}
	}
	return true, strings.TrimSpace(b.String()), nil
}

func (s *Store) upsertMemoryFromFeedback(ctx context.Context, row ContentFeedbackRow) error {
	memoryType, prefix := feedbackMemoryMapping(row.Rating)
	if memoryType == "" {
		return nil
	}
	contextText, err := s.memoryContextForTarget(ctx, row.UserID, row.TargetType, row.TargetID)
	if err != nil {
		contextText = row.TargetID
	}
	parts := []string{prefix + contextText}
	if row.Intent != "" {
		parts = append(parts, "用户意图："+row.Intent)
	}
	if row.Comment != "" {
		parts = append(parts, "补充反馈："+row.Comment)
	}
	_, err = s.UpsertUserMemory(ctx, row.UserID, UserMemoryInput{
		MemoryType: memoryType,
		Content:    strings.Join(parts, "\n"),
		Confidence: feedbackConfidence(row.Rating),
		SourceType: "feedback",
		SourceID:   row.ID,
	})
	return err
}

func (s *Store) memoryContextForTarget(ctx context.Context, userID, targetType, targetID string) (string, error) {
	if targetType != "article" {
		return targetType + " " + targetID, nil
	}
	var title, url, summary, category string
	var tags []string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(title, ''),
		       COALESCE(url, ''),
		       COALESCE(summary, ''),
		       COALESCE(category, ''),
		       COALESCE(tags, '{}'::text[])
		FROM articles
		WHERE user_id = $1 AND id = $2`, userID, targetID).Scan(&title, &url, &summary, &category, &tags)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		return "", err
	}
	parts := []string{}
	if title != "" {
		parts = append(parts, "标题："+title)
	}
	if category != "" {
		parts = append(parts, "分类："+category)
	}
	if len(tags) > 0 {
		parts = append(parts, "标签："+strings.Join(tags, "、"))
	}
	if summary != "" {
		parts = append(parts, "摘要："+truncateText(summary, 260))
	}
	if url != "" {
		parts = append(parts, "链接："+url)
	}
	return truncateText(strings.Join(parts, "\n"), 620), nil
}

func feedbackMemoryMapping(rating string) (string, string) {
	switch rating {
	case feedbackUseful:
		return memoryInterest, "用户认为这类内容有用："
	case feedbackNotifySimilar:
		return memoryNotify, "以后遇到类似内容应优先通知："
	case feedbackSilentSimilar:
		return memorySilent, "以后遇到类似内容可以静默保存："
	case feedbackNotUseful, feedbackDiscardSimilar:
		return memoryReject, "用户对这类内容兴趣较低或希望降低优先级："
	default:
		return "", ""
	}
}

func feedbackConfidence(rating string) float64 {
	switch rating {
	case feedbackNotifySimilar, feedbackSilentSimilar, feedbackDiscardSimilar:
		return 0.88
	case feedbackUseful, feedbackNotUseful:
		return 0.74
	default:
		return 0.6
	}
}

func normalizeFeedbackInput(input FeedbackInput) FeedbackInput {
	input.TargetType = trimMax(strings.ToLower(strings.TrimSpace(input.TargetType)), 40)
	input.TargetID = trimMax(strings.TrimSpace(input.TargetID), 160)
	input.Rating = strings.ToLower(strings.TrimSpace(input.Rating))
	switch input.Rating {
	case feedbackUseful, feedbackNotUseful, feedbackNotifySimilar, feedbackSilentSimilar, feedbackDiscardSimilar:
	default:
		input.Rating = ""
	}
	input.Intent = trimMax(input.Intent, 420)
	input.Comment = trimMax(input.Comment, 420)
	input.Source = trimMax(strings.ToLower(strings.TrimSpace(input.Source)), 40)
	if input.Source == "" {
		input.Source = "manual"
	}
	return input
}

func normalizeMemoryInput(input UserMemoryInput) UserMemoryInput {
	input.MemoryType = strings.ToLower(strings.TrimSpace(input.MemoryType))
	switch input.MemoryType {
	case memoryInterest, memoryNotify, memorySilent, memoryReject, memoryIntent:
	default:
		input.MemoryType = memoryInterest
	}
	input.Content = trimMax(input.Content, 900)
	if input.Confidence <= 0 {
		input.Confidence = 0.6
	}
	if input.Confidence > 1 {
		input.Confidence = 1
	}
	input.SourceType = trimMax(strings.ToLower(strings.TrimSpace(input.SourceType)), 40)
	input.SourceID = trimMax(strings.TrimSpace(input.SourceID), 160)
	return input
}

func normalizeMemoryList(values []string, limit int) []string {
	out := make([]string, 0, minInt(limit, len(values)))
	for _, value := range values {
		value = memoryBrief(value)
		if value == "" {
			continue
		}
		out = appendUniqueLimited(out, value, limit)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func appendUniqueLimited(values []string, value string, limit int) []string {
	value = memoryBrief(value)
	if value == "" || len(values) >= limit {
		return values
	}
	key := strings.ToLower(value)
	for _, existing := range values {
		if strings.ToLower(existing) == key {
			return values
		}
	}
	return append(values, value)
}

func memoryBrief(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return truncateText(value, 180)
}

func writePromptList(b *strings.Builder, title string, values []string, limit int) {
	if len(values) == 0 {
		return
	}
	b.WriteString(title)
	b.WriteString("：\n")
	for i, value := range values {
		if i >= limit {
			break
		}
		fmt.Fprintf(b, "- %s\n", memoryBrief(value))
	}
}

func normalizeMemoryLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 120 {
		return 120
	}
	return limit
}

func (s *Store) ensurePreferenceProfile(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO preference_profiles (user_id, memory_enabled, profile_json, version, created_at, updated_at)
		VALUES ($1, TRUE, '{}'::jsonb, 1, NOW(), NOW())
		ON CONFLICT (user_id) DO NOTHING`, userID)
	return err
}

func profileFromRaw(userID string, enabled bool, raw string, version int, updatedAt time.Time) (PreferenceProfile, error) {
	payload := preferenceProfilePayload{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return PreferenceProfile{}, err
		}
	}
	return PreferenceProfile{
		UserID:             userID,
		MemoryEnabled:      enabled,
		Interests:          normalizeMemoryList(payload.Interests, 12),
		NotifyPreferences:  normalizeMemoryList(payload.NotifyPreferences, 12),
		ArchivePreferences: normalizeMemoryList(payload.ArchivePreferences, 12),
		RejectPatterns:     normalizeMemoryList(payload.RejectPatterns, 12),
		RecentIntents:      normalizeMemoryList(payload.RecentIntents, 12),
		FeedbackCount:      payload.FeedbackCount,
		MemoryCount:        payload.MemoryCount,
		Version:            version,
		UpdatedAt:          updatedAt,
	}, nil
}

func scanFeedback(row scanner) (ContentFeedbackRow, error) {
	var feedback ContentFeedbackRow
	err := row.Scan(
		&feedback.ID,
		&feedback.UserID,
		&feedback.TargetType,
		&feedback.TargetID,
		&feedback.Rating,
		&feedback.Intent,
		&feedback.Comment,
		&feedback.Source,
		&feedback.CreatedAt,
		&feedback.UpdatedAt,
	)
	return feedback, err
}

func scanMemory(row scanner) (UserMemoryRow, error) {
	var memory UserMemoryRow
	var disabledAt pgtype.Timestamptz
	err := row.Scan(
		&memory.ID,
		&memory.UserID,
		&memory.MemoryType,
		&memory.Content,
		&memory.Confidence,
		&memory.SourceType,
		&memory.SourceID,
		&disabledAt,
		&memory.CreatedAt,
		&memory.UpdatedAt,
	)
	if err != nil {
		return memory, err
	}
	if disabledAt.Valid {
		t := disabledAt.Time
		memory.DisabledAt = &t
	}
	return memory, nil
}

func scanMemoryRows(rows pgx.Rows) ([]UserMemoryRow, error) {
	var memories []UserMemoryRow
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if memories == nil {
		return []UserMemoryRow{}, nil
	}
	return memories, nil
}
