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
		referer:        defaultYTDLPReferer,
		acceptLanguage: defaultAcceptLanguage,
	}
	got := strings.Join(f.baseYTDLPArgs(), "\n")
	for _, want := range []string{
		"--no-warnings",
		"--user-agent\n" + defaultYTDLPUserAgent,
		"--referer\n" + defaultYTDLPReferer,
		"--add-header\nAccept-Language:" + defaultAcceptLanguage,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in args: %q", want, got)
		}
	}
	if strings.Contains(got, "--no-call-home") {
		t.Fatalf("deprecated --no-call-home should not be used: %q", got)
	}
}
