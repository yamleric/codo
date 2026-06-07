package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const runtimeConfigKey = "runtime"

type AuthStatus struct {
	SetupRequired bool   `json:"setup_required"`
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
}

type AuthUser struct {
	UserID       string
	Username     string
	PasswordHash string
}

func (s *Store) AuthStatus(ctx context.Context, userID string) (AuthStatus, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return AuthStatus{}, err
	}
	var status AuthStatus
	status.UserID = userID
	var passwordHash string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(username, ''), COALESCE(password_hash, '')
		FROM users
		WHERE id = $1`, userID).Scan(&status.Username, &passwordHash)
	if err != nil {
		return AuthStatus{}, err
	}
	status.SetupRequired = strings.TrimSpace(passwordHash) == ""
	return status, nil
}

func (s *Store) SetupOwner(ctx context.Context, userID, username, passwordHash string) (AuthUser, error) {
	if err := s.ensureUser(ctx, userID); err != nil {
		return AuthUser{}, err
	}
	username = normalizeUsername(username)
	if username == "" {
		return AuthUser{}, fmt.Errorf("username is required")
	}
	var user AuthUser
	err := s.db.QueryRow(ctx, `
		UPDATE users
		SET username = $2,
		    password_hash = $3,
		    auth_enabled = TRUE,
		    updated_at = NOW()
		WHERE id = $1 AND COALESCE(password_hash, '') = ''
		RETURNING id, username, password_hash`,
		userID, username, passwordHash).Scan(&user.UserID, &user.Username, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthUser{}, fmt.Errorf("setup already completed")
		}
		return AuthUser{}, err
	}
	return user, nil
}

func (s *Store) FindAuthUser(ctx context.Context, username string) (AuthUser, error) {
	username = normalizeUsername(username)
	var user AuthUser
	err := s.db.QueryRow(ctx, `
		SELECT id, username, password_hash
		FROM users
		WHERE lower(username) = lower($1)
		  AND COALESCE(password_hash, '') <> ''
		LIMIT 1`, username).Scan(&user.UserID, &user.Username, &user.PasswordHash)
	if err != nil {
		return AuthUser{}, err
	}
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	id := fmt.Sprintf("session-%d", time.Now().UnixNano())
	_, err := s.db.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		id, userID, tokenHash, expiresAt)
	return err
}

func (s *Store) SessionUser(ctx context.Context, tokenHash string) (AuthUser, error) {
	var user AuthUser
	err := s.db.QueryRow(ctx, `
		UPDATE auth_sessions
		SET last_seen_at = NOW()
		WHERE token_hash = $1 AND expires_at > NOW()
		RETURNING user_id`, tokenHash).Scan(&user.UserID)
	if err != nil {
		return AuthUser{}, err
	}
	err = s.db.QueryRow(ctx, `
		SELECT id, username, password_hash
		FROM users
		WHERE id = $1 AND COALESCE(password_hash, '') <> ''`, user.UserID).Scan(&user.UserID, &user.Username, &user.PasswordHash)
	if err != nil {
		return AuthUser{}, err
	}
	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM auth_sessions WHERE expires_at <= NOW()`)
	return err
}

type AppConfig struct {
	LLM       LLMRuntimeConfig      `json:"llm"`
	Embedding LLMRuntimeConfig      `json:"embedding"`
	ASR       ASRRuntimeConfig      `json:"asr"`
	Telegram  TelegramRuntimeConfig `json:"telegram"`
	SMTP      SMTPRuntimeConfig     `json:"smtp"`
}

type LLMRuntimeConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type ASRRuntimeConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type TelegramRuntimeConfig struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

type SMTPRuntimeConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		LLM:       LLMRuntimeConfig{BaseURL: "", Model: ""},
		Embedding: LLMRuntimeConfig{BaseURL: "", Model: ""},
		ASR:       ASRRuntimeConfig{BaseURL: "", Model: ""},
		SMTP:      SMTPRuntimeConfig{Port: 587},
	}
}

func (s *Store) GetAppConfig(ctx context.Context) (AppConfig, error) {
	config := DefaultAppConfig()
	var raw string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(value, '{}'::jsonb)::text
		FROM app_settings
		WHERE key = $1`, runtimeConfigKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return config, nil
		}
		return AppConfig{}, err
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return AppConfig{}, fmt.Errorf("parse app config: %w", err)
		}
	}
	return NormalizeAppConfig(config), nil
}

func (s *Store) SaveAppConfig(ctx context.Context, config AppConfig) error {
	config = NormalizeAppConfig(config)
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = NOW()`,
		runtimeConfigKey, string(payload))
	return err
}

func NormalizeAppConfig(config AppConfig) AppConfig {
	config.LLM = normalizeLLMRuntime(config.LLM)
	config.Embedding = normalizeLLMRuntime(config.Embedding)
	config.ASR.BaseURL = strings.TrimSpace(config.ASR.BaseURL)
	config.ASR.APIKey = strings.TrimSpace(config.ASR.APIKey)
	config.ASR.Model = trimMax(config.ASR.Model, 120)
	config.Telegram.Token = strings.TrimSpace(config.Telegram.Token)
	config.Telegram.ChatID = trimMax(config.Telegram.ChatID, 80)
	config.SMTP.Host = trimMax(config.SMTP.Host, 160)
	config.SMTP.Username = trimMax(config.SMTP.Username, 160)
	config.SMTP.Password = strings.TrimSpace(config.SMTP.Password)
	config.SMTP.From = trimMax(config.SMTP.From, 160)
	if config.SMTP.Port <= 0 {
		config.SMTP.Port = 587
	}
	if config.SMTP.Port > 65535 {
		config.SMTP.Port = 587
	}
	return config
}

func MergeAppConfig(primary, fallback AppConfig) AppConfig {
	primary = NormalizeAppConfig(primary)
	fallback = NormalizeAppConfig(fallback)
	if primary.LLM.BaseURL == "" {
		primary.LLM.BaseURL = fallback.LLM.BaseURL
	}
	if primary.LLM.APIKey == "" {
		primary.LLM.APIKey = fallback.LLM.APIKey
	}
	if primary.LLM.Model == "" {
		primary.LLM.Model = fallback.LLM.Model
	}
	if primary.Embedding.BaseURL == "" {
		primary.Embedding.BaseURL = fallback.Embedding.BaseURL
	}
	if primary.Embedding.APIKey == "" {
		primary.Embedding.APIKey = fallback.Embedding.APIKey
	}
	if primary.Embedding.Model == "" {
		primary.Embedding.Model = fallback.Embedding.Model
	}
	if primary.ASR.BaseURL == "" {
		primary.ASR.BaseURL = fallback.ASR.BaseURL
	}
	if primary.ASR.APIKey == "" {
		primary.ASR.APIKey = fallback.ASR.APIKey
	}
	if primary.ASR.Model == "" {
		primary.ASR.Model = fallback.ASR.Model
	}
	if primary.Telegram.Token == "" {
		primary.Telegram.Token = fallback.Telegram.Token
	}
	if primary.Telegram.ChatID == "" {
		primary.Telegram.ChatID = fallback.Telegram.ChatID
	}
	if primary.SMTP.Host == "" {
		primary.SMTP.Host = fallback.SMTP.Host
	}
	if primary.SMTP.Username == "" {
		primary.SMTP.Username = fallback.SMTP.Username
	}
	if primary.SMTP.Password == "" {
		primary.SMTP.Password = fallback.SMTP.Password
	}
	if primary.SMTP.From == "" {
		primary.SMTP.From = fallback.SMTP.From
	}
	if primary.SMTP.Port == 587 && fallback.SMTP.Port != 0 {
		primary.SMTP.Port = fallback.SMTP.Port
	}
	if !primary.SMTP.UseTLS {
		primary.SMTP.UseTLS = fallback.SMTP.UseTLS
	}
	return NormalizeAppConfig(primary)
}

func normalizeLLMRuntime(config LLMRuntimeConfig) LLMRuntimeConfig {
	config.BaseURL = trimMax(config.BaseURL, 240)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = trimMax(config.Model, 120)
	return config
}

func normalizeUsername(username string) string {
	return trimMax(username, 64)
}

func trimMax(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}
