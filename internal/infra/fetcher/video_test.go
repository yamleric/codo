package fetcher

import (
	"strings"
	"testing"
)

func TestExtractSubtitleTextFromVTT(t *testing.T) {
	raw := []byte(`WEBVTT

00:00:00.000 --> 00:00:02.000
大家好，今天讲 Go 的并发模型。

00:00:02.000 --> 00:00:04.000
goroutine 很轻量。
`)
	got := extractSubtitleText("subtitle.zh.vtt", raw)
	if !strings.Contains(got, "大家好") || !strings.Contains(got, "goroutine 很轻量") {
		t.Fatalf("unexpected transcript: %q", got)
	}
	if strings.Contains(got, "-->") || strings.Contains(got, "WEBVTT") {
		t.Fatalf("timestamp/header leaked: %q", got)
	}
}

func TestExtractSubtitleTextFromBilibiliJSON(t *testing.T) {
	raw := []byte(`{"body":[{"from":0,"to":1,"content":"第一段字幕"},{"from":1,"to":2,"content":"第二段字幕"}]}`)
	got := extractSubtitleText("subtitle.zh.json", raw)
	if got != "第一段字幕\n第二段字幕" {
		t.Fatalf("unexpected transcript: %q", got)
	}
}

func TestExtractSubtitleTextFromJSON3(t *testing.T) {
	raw := []byte(`{"events":[{"segs":[{"utf8":"Hello "},{"utf8":"world"}]},{"segs":[{"utf8":"Next line"}]}]}`)
	got := extractSubtitleText("subtitle.en.json3", raw)
	if got != "Hello\nworld\nNext line" {
		t.Fatalf("unexpected transcript: %q", got)
	}
}

func TestComposeVideoContentIncludesMetadata(t *testing.T) {
	got := composeVideoContent(ytDLPInfo{
		Title:        "测试视频",
		Uploader:     "作者",
		Duration:     125,
		WebpageURL:   "https://www.bilibili.com/video/BV1xx411c7mD/",
		ExtractorKey: "BiliBili",
	}, "字幕", "核心内容")

	for _, want := range []string{"标题：测试视频", "作者：作者", "时长：2:05", "文字稿来源：字幕", "核心内容"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestBaseYTDLPArgsUseBrowserHeaders(t *testing.T) {
	f := &VideoFetcher{
		userAgent:      defaultYTDLPUserAgent,
		acceptLanguage: defaultAcceptLanguage,
	}
	got := strings.Join(f.baseYTDLPArgs("https://www.bilibili.com/video/BV1xx411c7mD/"), "\n")
	for _, want := range []string{
		"--no-warnings",
		"--user-agent\n" + defaultYTDLPUserAgent,
		"--referer\n" + defaultBilibiliReferer,
		"--add-header\nAccept-Language:" + defaultAcceptLanguage,
		"--add-header\nAccept:text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		"--add-header\nSec-CH-UA-Platform:\"Windows\"",
		"--add-header\nUpgrade-Insecure-Requests:1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in args: %q", want, got)
		}
	}
	if strings.Contains(got, "--no-call-home") {
		t.Fatalf("deprecated --no-call-home should not be used: %q", got)
	}
}

func TestBaseYTDLPArgsUseDouyinReferer(t *testing.T) {
	f := &VideoFetcher{
		userAgent:      defaultYTDLPUserAgent,
		acceptLanguage: defaultAcceptLanguage,
	}
	got := strings.Join(f.baseYTDLPArgs("https://v.douyin.com/iAbCdEf/"), "\n")
	if !strings.Contains(got, "--referer\n"+defaultDouyinReferer) {
		t.Fatalf("missing douyin referer in args: %q", got)
	}
	if strings.Contains(got, defaultBilibiliReferer) {
		t.Fatalf("bilibili referer should not be used for douyin: %q", got)
	}
}

func TestBaseYTDLPArgsAllowRefererOverride(t *testing.T) {
	f := &VideoFetcher{
		userAgent:      defaultYTDLPUserAgent,
		referer:        "https://example.com/",
		acceptLanguage: defaultAcceptLanguage,
	}
	got := strings.Join(f.baseYTDLPArgs("https://v.douyin.com/iAbCdEf/"), "\n")
	if !strings.Contains(got, "--referer\nhttps://example.com/") {
		t.Fatalf("missing override referer in args: %q", got)
	}
}

func TestBaseYTDLPArgsUseBrowserCookies(t *testing.T) {
	f := &VideoFetcher{
		userAgent:          defaultYTDLPUserAgent,
		acceptLanguage:     defaultAcceptLanguage,
		cookiesFromBrowser: "chrome:/home/ubuntu/.config/google-chrome",
	}
	got := strings.Join(f.baseYTDLPArgs("https://www.douyin.com/video/123"), "\n")
	if !strings.Contains(got, "--cookies-from-browser\nchrome:/home/ubuntu/.config/google-chrome") {
		t.Fatalf("missing browser cookies arg: %q", got)
	}
}

func TestBaseYTDLPArgsPreferCookiesFile(t *testing.T) {
	f := &VideoFetcher{
		userAgent:          defaultYTDLPUserAgent,
		acceptLanguage:     defaultAcceptLanguage,
		cookiesFile:        "/run/secrets/yt-dlp.cookies.txt",
		cookiesFromBrowser: "chrome:/home/ubuntu/.config/google-chrome",
	}
	got := strings.Join(f.baseYTDLPArgs("https://www.douyin.com/video/123"), "\n")
	if !strings.Contains(got, "--cookies\n/run/secrets/yt-dlp.cookies.txt") {
		t.Fatalf("missing cookies file arg: %q", got)
	}
	if strings.Contains(got, "--cookies-from-browser") {
		t.Fatalf("browser cookies should not be used when cookies file is set: %q", got)
	}
}

func TestNormalizeYTDLPErrorExplainsDouyinCookies(t *testing.T) {
	err := normalizeYTDLPError(
		"https://www.douyin.com/video/7646064198796108537",
		assertErr("yt-dlp: ERROR: [Douyin] 7646064198796108537: Fresh cookies (not necessarily logged in) are needed"),
	)
	got := err.Error()
	if !strings.Contains(got, "抖音当前要求 fresh cookies") ||
		!strings.Contains(got, "YTDLP_COOKIES_FILE") ||
		!strings.Contains(got, "YTDLP_COOKIES_FROM_BROWSER") {
		t.Fatalf("unexpected normalized error: %q", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
