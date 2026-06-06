package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
)

const (
	defaultSubtitleLanguages = "zh.*,zh-Hans,zh-Hant,zh-CN,zh-TW,cmn.*,yue.*,en.*"
	defaultASRModel          = "whisper-1"
	defaultASRChunkSeconds   = 600
	defaultMaxAudioBytes     = 24 * 1024 * 1024
	defaultMaxDurationSec    = 2 * 60 * 60
	defaultYTDLPUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36"
	defaultYTDLPReferer      = "https://www.bilibili.com/"
	defaultAcceptLanguage    = "zh-CN,zh;q=0.9,en;q=0.8"
	minTranscriptRunes       = 40
)

var (
	subtitleTimestamp = regexp.MustCompile(`^\s*(\d{1,2}:)?\d{1,2}:\d{2}[,.]\d{1,3}\s+-->\s+`)
	htmlTag           = regexp.MustCompile(`<[^>]+>`)
)

// VideoFetcher extracts a transcript from public video URLs using yt-dlp.
// It prefers platform subtitles and falls back to audio transcription when an
// OpenAI-compatible audio transcription endpoint is configured.
type VideoFetcher struct {
	ytDLP             string
	ffmpeg            string
	cookiesFile       string
	userAgent         string
	referer           string
	acceptLanguage    string
	subtitleLanguages string
	asrBaseURL        string
	asrAPIKey         string
	asrModel          string
	asrChunkSeconds   int
	maxAudioBytes     int64
	maxDurationSec    int
	http              *http.Client
}

type ytDLPInfo struct {
	ID                string                    `json:"id"`
	Title             string                    `json:"title"`
	Uploader          string                    `json:"uploader"`
	Channel           string                    `json:"channel"`
	Duration          float64                   `json:"duration"`
	Description       string                    `json:"description"`
	WebpageURL        string                    `json:"webpage_url"`
	ExtractorKey      string                    `json:"extractor_key"`
	Subtitles         map[string][]ytDLPSubInfo `json:"subtitles"`
	AutomaticCaptions map[string][]ytDLPSubInfo `json:"automatic_captions"`
	Entries           []ytDLPInfo               `json:"entries"`
}

type ytDLPSubInfo struct {
	URL  string `json:"url"`
	Ext  string `json:"ext"`
	Name string `json:"name"`
}

// NewVideo creates a VideoFetcher from environment variables.
func NewVideo() *VideoFetcher {
	return &VideoFetcher{
		ytDLP:             getenvDefault("YTDLP_BIN", "yt-dlp"),
		ffmpeg:            getenvDefault("FFMPEG_BIN", "ffmpeg"),
		cookiesFile:       os.Getenv("YTDLP_COOKIES_FILE"),
		userAgent:         getenvDefault("YTDLP_USER_AGENT", defaultYTDLPUserAgent),
		referer:           getenvDefault("YTDLP_REFERER", defaultYTDLPReferer),
		acceptLanguage:    getenvDefault("YTDLP_ACCEPT_LANGUAGE", defaultAcceptLanguage),
		subtitleLanguages: getenvDefault("VIDEO_SUB_LANGS", defaultSubtitleLanguages),
		asrBaseURL:        getenvDefault("ASR_BASE_URL", getenvDefault("LLM_BASE_URL", "https://api.openai.com/v1")),
		asrAPIKey:         getenvDefault("ASR_API_KEY", os.Getenv("LLM_API_KEY")),
		asrModel:          getenvDefault("ASR_MODEL", defaultASRModel),
		asrChunkSeconds:   getenvInt("ASR_CHUNK_SECONDS", defaultASRChunkSeconds),
		maxAudioBytes:     int64(getenvInt("ASR_MAX_AUDIO_BYTES", defaultMaxAudioBytes)),
		maxDurationSec:    getenvInt("VIDEO_MAX_DURATION_SECONDS", defaultMaxDurationSec),
		http:              &http.Client{Timeout: 10 * time.Minute},
	}
}

func (f *VideoFetcher) Fetch(ctx context.Context, t *task.Task) (string, error) {
	if t.URL == "" {
		return "", fmt.Errorf("video fetcher: no URL")
	}

	workDir, err := os.MkdirTemp("", "codo-video-*")
	if err != nil {
		return "", fmt.Errorf("video fetcher: temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	info, err := f.probe(ctx, t.URL)
	if err != nil {
		return "", err
	}
	if info.Duration > 0 && f.maxDurationSec > 0 && int(info.Duration) > f.maxDurationSec {
		return "", fmt.Errorf("video fetcher: video too long (%s > %s)",
			formatDuration(info.Duration), formatDuration(float64(f.maxDurationSec)))
	}

	if transcript, err := f.downloadSubtitles(ctx, workDir, t.URL); err == nil && enoughTranscript(transcript) {
		return composeVideoContent(info, "字幕", transcript), nil
	}

	var asrErr error
	if f.asrConfigured() {
		audio, err := f.downloadAudio(ctx, workDir, t.URL)
		if err != nil {
			asrErr = err
		} else {
			transcript, err := f.transcribeAudio(ctx, workDir, audio)
			if err != nil {
				asrErr = err
			} else if enoughTranscript(transcript) {
				return composeVideoContent(info, "语音转文字", transcript), nil
			}
		}
	}

	fallback := metadataFallback(info)
	if enoughTranscript(fallback) {
		return composeVideoContent(info, "视频简介", fallback), nil
	}
	if asrErr != nil {
		return "", asrErr
	}
	return "", fmt.Errorf("video fetcher: no subtitles found and ASR is not configured")
}

func (f *VideoFetcher) probe(ctx context.Context, rawURL string) (ytDLPInfo, error) {
	args := f.baseYTDLPArgs()
	args = append(args, "--dump-single-json", "--skip-download", "--no-playlist", rawURL)
	out, err := runCommand(ctx, f.ytDLP, args...)
	if err != nil {
		return ytDLPInfo{}, fmt.Errorf("video fetcher: yt-dlp metadata: %w", err)
	}

	var info ytDLPInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return ytDLPInfo{}, fmt.Errorf("video fetcher: parse metadata: %w", err)
	}
	if len(info.Entries) > 0 && info.Title == "" {
		info = info.Entries[0]
	}
	return info, nil
}

func (f *VideoFetcher) downloadSubtitles(ctx context.Context, workDir, rawURL string) (string, error) {
	args := f.baseYTDLPArgs()
	args = append(args,
		"--skip-download",
		"--no-playlist",
		"--write-subs",
		"--write-auto-subs",
		"--sub-langs", f.subtitleLanguages,
		"--sub-format", "vtt/srt/json3/json/ttml/best",
		"--paths", workDir,
		"-o", "subtitle.%(ext)s",
		rawURL,
	)
	_, _ = runCommand(ctx, f.ytDLP, args...)

	files, err := subtitleFiles(workDir)
	if err != nil {
		return "", err
	}

	type candidate struct {
		text  string
		score int
	}
	candidates := make([]candidate, 0, len(files))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		text := extractSubtitleText(filepath.Base(file), raw)
		if !enoughTranscript(text) {
			continue
		}
		score := len([]rune(text))
		if likelyChineseSubtitle(file, text) {
			score += 1_000_000
		}
		candidates = append(candidates, candidate{text: text, score: score})
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("video fetcher: no usable subtitles")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	return candidates[0].text, nil
}

func (f *VideoFetcher) downloadAudio(ctx context.Context, workDir, rawURL string) (string, error) {
	args := f.baseYTDLPArgs()
	args = append(args,
		"--no-playlist",
		"-f", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "64K",
		"--paths", workDir,
		"-o", "audio.%(ext)s",
		rawURL,
	)
	if _, err := runCommand(ctx, f.ytDLP, args...); err != nil {
		return "", fmt.Errorf("video fetcher: download audio: %w", err)
	}

	matches, _ := filepath.Glob(filepath.Join(workDir, "audio.*"))
	if len(matches) == 0 {
		return "", fmt.Errorf("video fetcher: audio file not produced")
	}
	sort.Slice(matches, func(i, j int) bool {
		li, _ := fileSize(matches[i])
		lj, _ := fileSize(matches[j])
		return li > lj
	})
	return matches[0], nil
}

func (f *VideoFetcher) transcribeAudio(ctx context.Context, workDir, audioPath string) (string, error) {
	chunks, err := f.chunkAudio(ctx, workDir, audioPath)
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		size, err := fileSize(chunk)
		if err != nil {
			return "", err
		}
		if size > f.maxAudioBytes {
			return "", fmt.Errorf("video fetcher: ASR chunk too large (%d bytes)", size)
		}
		text, err := f.transcribeFile(ctx, chunk)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("video fetcher: ASR returned empty transcript")
	}
	return strings.Join(parts, "\n\n"), nil
}

func (f *VideoFetcher) chunkAudio(ctx context.Context, workDir, audioPath string) ([]string, error) {
	if f.asrChunkSeconds <= 0 {
		return []string{audioPath}, nil
	}
	chunkPattern := filepath.Join(workDir, "chunk-%03d.mp3")
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", audioPath,
		"-f", "segment",
		"-segment_time", strconv.Itoa(f.asrChunkSeconds),
		"-ar", "16000",
		"-ac", "1",
		"-b:a", "48k",
		"-c:a", "libmp3lame",
		chunkPattern,
	}
	if _, err := runCommand(ctx, f.ffmpeg, args...); err != nil {
		size, statErr := fileSize(audioPath)
		if statErr == nil && size <= f.maxAudioBytes {
			return []string{audioPath}, nil
		}
		return nil, fmt.Errorf("video fetcher: split audio: %w", err)
	}
	chunks, _ := filepath.Glob(filepath.Join(workDir, "chunk-*.mp3"))
	if len(chunks) == 0 {
		return nil, fmt.Errorf("video fetcher: no audio chunks produced")
	}
	sort.Strings(chunks)
	return chunks, nil
}

func (f *VideoFetcher) transcribeFile(ctx context.Context, audioPath string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", f.asrModel); err != nil {
		return "", err
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", err
	}

	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	file, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		file.Close()
		return "", err
	}
	file.Close()
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(f.asrBaseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+f.asrAPIKey)

	resp, err := f.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("asr request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("asr http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parse asr response: %w", err)
	}
	return payload.Text, nil
}

func (f *VideoFetcher) baseYTDLPArgs() []string {
	args := []string{
		"--no-warnings",
	}
	if f.userAgent != "" {
		args = append(args, "--user-agent", f.userAgent)
	}
	if f.referer != "" {
		args = append(args, "--referer", f.referer)
	}
	if f.acceptLanguage != "" {
		args = append(args, "--add-header", "Accept-Language:"+f.acceptLanguage)
	}
	if f.cookiesFile != "" {
		args = append(args, "--cookies", f.cookiesFile)
	}
	return args
}

func (f *VideoFetcher) asrConfigured() bool {
	return f.asrBaseURL != "" && f.asrAPIKey != "" && f.asrModel != ""
}

func subtitleFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		switch filepath.Ext(name) {
		case ".vtt", ".srt", ".json", ".json3", ".ttml", ".xml", ".ass", ".ssa":
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("video fetcher: no subtitle files")
	}
	return files, nil
}

func extractSubtitleText(filename string, raw []byte) string {
	text := strings.TrimSpace(string(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	if text == "" {
		return ""
	}
	first := text[0]
	if first == '{' || first == '[' || strings.HasSuffix(strings.ToLower(filename), ".json3") {
		if parsed := extractJSONSubtitleText([]byte(text)); parsed != "" {
			return parsed
		}
	}
	return extractTimedText(text)
}

func extractJSONSubtitleText(raw []byte) string {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	var parts []string
	collectSubtitleStrings(payload, "", &parts)
	return normalizeSubtitleLines(parts)
}

func collectSubtitleStrings(v any, key string, parts *[]string) {
	switch current := v.(type) {
	case map[string]any:
		for k, value := range current {
			lower := strings.ToLower(k)
			if s, ok := value.(string); ok && isSubtitleTextKey(lower) {
				*parts = append(*parts, s)
				continue
			}
			if isSubtitleContainerKey(lower) {
				collectSubtitleStrings(value, lower, parts)
			}
		}
	case []any:
		for _, item := range current {
			collectSubtitleStrings(item, key, parts)
		}
	}
}

func isSubtitleTextKey(key string) bool {
	switch key {
	case "content", "text", "utf8", "words":
		return true
	default:
		return false
	}
}

func isSubtitleContainerKey(key string) bool {
	switch key {
	case "body", "events", "segs", "segments", "captions", "subtitles":
		return true
	default:
		return false
	}
}

func extractTimedText(text string) string {
	lines := strings.Split(text, "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" ||
			line == "WEBVTT" ||
			line == "[Script Info]" ||
			line == "[V4+ Styles]" ||
			line == "[Events]" ||
			strings.HasPrefix(line, "NOTE") ||
			strings.HasPrefix(line, "STYLE") ||
			strings.HasPrefix(line, "Kind:") ||
			strings.HasPrefix(line, "Language:") ||
			subtitleTimestamp.MatchString(line) ||
			isInteger(line) {
			continue
		}
		if strings.Contains(line, "Dialogue:") {
			line = assDialogueText(line)
		}
		line = cleanSubtitleLine(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return normalizeSubtitleLines(parts)
}

func assDialogueText(line string) string {
	parts := strings.SplitN(line, ",", 10)
	if len(parts) == 10 {
		return parts[9]
	}
	return line
}

func cleanSubtitleLine(line string) string {
	line = htmlTag.ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, `\N`, " ")
	line = strings.ReplaceAll(line, `\n`, " ")
	line = strings.ReplaceAll(line, "&nbsp;", " ")
	line = html.UnescapeString(line)
	line = whitespace.ReplaceAllString(line, " ")
	return strings.TrimSpace(line)
}

func normalizeSubtitleLines(lines []string) string {
	out := make([]string, 0, len(lines))
	last := ""
	for _, line := range lines {
		line = cleanSubtitleLine(line)
		if line == "" || line == last {
			continue
		}
		out = append(out, line)
		last = line
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func likelyChineseSubtitle(path, text string) bool {
	name := strings.ToLower(filepath.Base(path))
	if strings.Contains(name, ".zh") ||
		strings.Contains(name, "zh-") ||
		strings.Contains(name, "zh_") ||
		strings.Contains(name, ".cmn") ||
		strings.Contains(name, ".yue") {
		return true
	}
	for _, r := range text {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func composeVideoContent(info ytDLPInfo, source, transcript string) string {
	var parts []string
	if info.Title != "" {
		parts = append(parts, "标题："+info.Title)
	}
	if uploader := firstNonEmpty(info.Uploader, info.Channel); uploader != "" {
		parts = append(parts, "作者："+uploader)
	}
	if info.ExtractorKey != "" {
		parts = append(parts, "平台："+info.ExtractorKey)
	}
	if info.Duration > 0 {
		parts = append(parts, "时长："+formatDuration(info.Duration))
	}
	if info.WebpageURL != "" {
		parts = append(parts, "来源："+info.WebpageURL)
	}
	parts = append(parts, "文字稿来源："+source)
	parts = append(parts, "文字稿：\n"+strings.TrimSpace(transcript))
	return strings.Join(parts, "\n\n")
}

func metadataFallback(info ytDLPInfo) string {
	var parts []string
	if info.Title != "" {
		parts = append(parts, info.Title)
	}
	if info.Description != "" {
		parts = append(parts, info.Description)
	}
	return strings.Join(parts, "\n\n")
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", name, msg)
	}
	return stdout.Bytes(), nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func enoughTranscript(text string) bool {
	return len([]rune(strings.TrimSpace(text))) >= minTranscriptRunes
}

func isInteger(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

var _ pipeline.Fetcher = (*VideoFetcher)(nil)
