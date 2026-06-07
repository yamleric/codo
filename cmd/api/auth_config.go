package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	authutil "github.com/codo/codo/internal/infra/auth"
	"github.com/codo/codo/internal/infra/store"
)

const (
	sessionCookieName = "codo_session"
	sessionTTL        = 30 * 24 * time.Hour
)

type authPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	SetupRequired bool   `json:"setup_required"`
	Authenticated bool   `json:"authenticated"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
}

func (s *server) authStatus(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := defaultUserID()
	status, err := s.st.AuthStatus(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	response := authResponse{
		SetupRequired: status.SetupRequired,
		UserID:        userID,
		Username:      status.Username,
	}
	if !status.SetupRequired {
		if user, ok := s.currentSessionUser(r); ok {
			response.Authenticated = true
			response.UserID = user.UserID
			response.Username = user.Username
		}
	}
	writeJSON(w, response)
}

func (s *server) authSetup(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body authPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(body.Username)
	password := strings.TrimSpace(body.Password)
	if username == "" || len([]rune(username)) > 64 {
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}
	if len([]rune(password)) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	hash, err := authutil.HashPassword(password)
	if err != nil {
		http.Error(w, "password hash failed", http.StatusInternalServerError)
		return
	}
	user, err := s.st.SetupOwner(r.Context(), defaultUserID(), username, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err := s.createSessionCookie(w, r, user.UserID); err != nil {
		http.Error(w, "session create failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, authResponse{
		SetupRequired: false,
		Authenticated: true,
		UserID:        user.UserID,
		Username:      user.Username,
	})
}

func (s *server) authLogin(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body authPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	user, err := s.st.FindAuthUser(r.Context(), body.Username)
	if err != nil || !authutil.VerifyPassword(body.Password, user.PasswordHash) {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	if err := s.createSessionCookie(w, r, user.UserID); err != nil {
		http.Error(w, "session create failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, authResponse{
		SetupRequired: false,
		Authenticated: true,
		UserID:        user.UserID,
		Username:      user.Username,
	})
}

func (s *server) authLogout(w http.ResponseWriter, r *http.Request) {
	setAuthHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = s.st.DeleteSession(r.Context(), authutil.HashToken(cookie.Value))
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		status, err := s.st.AuthStatus(r.Context(), defaultUserID())
		if err != nil {
			http.Error(w, "auth status failed", http.StatusInternalServerError)
			return
		}
		if status.SetupRequired {
			http.Error(w, "setup required", http.StatusPreconditionRequired)
			return
		}
		if _, ok := s.currentSessionUser(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) currentSessionUser(r *http.Request) (store.AuthUser, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return store.AuthUser{}, false
	}
	user, err := s.st.SessionUser(r.Context(), authutil.HashToken(cookie.Value))
	if err != nil {
		return store.AuthUser{}, false
	}
	return user, true
}

func (s *server) createSessionCookie(w http.ResponseWriter, r *http.Request, userID string) error {
	token, tokenHash, err := authutil.NewSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSession(r.Context(), userID, tokenHash, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie(r),
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secureCookie(r),
	})
}

func secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setAuthHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
}

func runtimeSettings(config store.AppConfig) settingsRuntime {
	_, ytDLPErr := exec.LookPath(getenv("YTDLP_BIN", "yt-dlp"))
	_, ffmpegErr := exec.LookPath(getenv("FFMPEG_BIN", "ffmpeg"))
	return settingsRuntime{
		LLMConfigured:          strings.TrimSpace(config.LLM.APIKey) != "",
		EmbeddingConfigured:    strings.TrimSpace(config.Embedding.APIKey) != "",
		ASRConfigured:          strings.TrimSpace(config.ASR.BaseURL) != "" && strings.TrimSpace(config.ASR.APIKey) != "",
		TelegramConfigured:     strings.TrimSpace(config.Telegram.Token) != "" && strings.TrimSpace(config.Telegram.ChatID) != "",
		EmailConfigured:        strings.TrimSpace(config.SMTP.Host) != "" && strings.TrimSpace(config.SMTP.From) != "",
		YTDLPConfigured:        ytDLPErr == nil,
		YTDLPCookiesSet:        os.Getenv("YTDLP_COOKIES_FILE") != "" || os.Getenv("YTDLP_COOKIES_FROM_BROWSER") != "",
		YTDLPBrowserCookiesSet: os.Getenv("YTDLP_COOKIES_FROM_BROWSER") != "",
		PlaywrightConfigured:   playwrightAvailable(),
		FFMPEGConfigured:       ffmpegErr == nil,
	}
}

type runtimeConfigResponse struct {
	LLM       serviceKeyResponse `json:"llm"`
	Embedding serviceKeyResponse `json:"embedding"`
	ASR       serviceKeyResponse `json:"asr"`
	Telegram  telegramResponse   `json:"telegram"`
	SMTP      smtpResponse       `json:"smtp"`
}

type serviceKeyResponse struct {
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	KeyConfigured bool   `json:"key_configured"`
}

type telegramResponse struct {
	ChatID          string `json:"chat_id"`
	TokenConfigured bool   `json:"token_configured"`
}

type smtpResponse struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	From               string `json:"from"`
	UseTLS             bool   `json:"use_tls"`
	PasswordConfigured bool   `json:"password_configured"`
}

func runtimeConfigResponseFromConfig(config store.AppConfig) runtimeConfigResponse {
	return runtimeConfigResponse{
		LLM: serviceKeyResponse{
			BaseURL:       config.LLM.BaseURL,
			Model:         config.LLM.Model,
			KeyConfigured: strings.TrimSpace(config.LLM.APIKey) != "",
		},
		Embedding: serviceKeyResponse{
			BaseURL:       config.Embedding.BaseURL,
			Model:         config.Embedding.Model,
			KeyConfigured: strings.TrimSpace(config.Embedding.APIKey) != "",
		},
		ASR: serviceKeyResponse{
			BaseURL:       config.ASR.BaseURL,
			Model:         config.ASR.Model,
			KeyConfigured: strings.TrimSpace(config.ASR.APIKey) != "",
		},
		Telegram: telegramResponse{
			ChatID:          config.Telegram.ChatID,
			TokenConfigured: strings.TrimSpace(config.Telegram.Token) != "",
		},
		SMTP: smtpResponse{
			Host:               config.SMTP.Host,
			Port:               config.SMTP.Port,
			Username:           config.SMTP.Username,
			From:               config.SMTP.From,
			UseTLS:             config.SMTP.UseTLS,
			PasswordConfigured: strings.TrimSpace(config.SMTP.Password) != "",
		},
	}
}

type runtimeConfigPatch struct {
	LLM       *serviceKeyPatch `json:"llm"`
	Embedding *serviceKeyPatch `json:"embedding"`
	ASR       *serviceKeyPatch `json:"asr"`
	Telegram  *telegramPatch   `json:"telegram"`
	SMTP      *smtpPatch       `json:"smtp"`
}

type serviceKeyPatch struct {
	BaseURL *string `json:"base_url"`
	Model   *string `json:"model"`
	APIKey  *string `json:"api_key"`
}

type telegramPatch struct {
	Token  *string `json:"token"`
	ChatID *string `json:"chat_id"`
}

type smtpPatch struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	From     *string `json:"from"`
	UseTLS   *bool   `json:"use_tls"`
}

func applyRuntimeConfigPatch(current store.AppConfig, patch *runtimeConfigPatch) (store.AppConfig, error) {
	if patch == nil {
		return current, nil
	}
	if patch.LLM != nil {
		current.LLM = applyServiceKeyPatch(current.LLM, patch.LLM)
	}
	if patch.Embedding != nil {
		current.Embedding = applyServiceKeyPatch(current.Embedding, patch.Embedding)
	}
	if patch.ASR != nil {
		if patch.ASR.BaseURL != nil {
			current.ASR.BaseURL = *patch.ASR.BaseURL
		}
		if patch.ASR.Model != nil {
			current.ASR.Model = *patch.ASR.Model
		}
		if patch.ASR.APIKey != nil {
			current.ASR.APIKey = *patch.ASR.APIKey
		}
	}
	if patch.Telegram != nil {
		if patch.Telegram.Token != nil {
			current.Telegram.Token = *patch.Telegram.Token
		}
		if patch.Telegram.ChatID != nil {
			current.Telegram.ChatID = *patch.Telegram.ChatID
		}
	}
	if patch.SMTP != nil {
		if patch.SMTP.Host != nil {
			current.SMTP.Host = *patch.SMTP.Host
		}
		if patch.SMTP.Port != nil {
			if *patch.SMTP.Port < 1 || *patch.SMTP.Port > 65535 {
				return store.AppConfig{}, fmt.Errorf("smtp.port must be between 1 and 65535")
			}
			current.SMTP.Port = *patch.SMTP.Port
		}
		if patch.SMTP.Username != nil {
			current.SMTP.Username = *patch.SMTP.Username
		}
		if patch.SMTP.Password != nil {
			current.SMTP.Password = *patch.SMTP.Password
		}
		if patch.SMTP.From != nil {
			current.SMTP.From = *patch.SMTP.From
		}
		if patch.SMTP.UseTLS != nil {
			current.SMTP.UseTLS = *patch.SMTP.UseTLS
		}
	}
	return store.NormalizeAppConfig(current), nil
}

func applyServiceKeyPatch(current store.LLMRuntimeConfig, patch *serviceKeyPatch) store.LLMRuntimeConfig {
	if patch.BaseURL != nil {
		current.BaseURL = *patch.BaseURL
	}
	if patch.Model != nil {
		current.Model = *patch.Model
	}
	if patch.APIKey != nil {
		current.APIKey = *patch.APIKey
	}
	return current
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func defaultUserID() string {
	return getenv("DEFAULT_USER_ID", "demo-user")
}
