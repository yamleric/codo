package ingest

import (
	"testing"

	"github.com/codo/codo/internal/domain/task"
)

func TestNormalizeURLAcceptsBareURL(t *testing.T) {
	got, err := NormalizeURL(" https://www.bilibili.com/video/BV1xx411c7mD/?spm_id_from=333 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.bilibili.com/video/BV1xx411c7mD/?spm_id_from=333" {
		t.Fatalf("unexpected url: %q", got)
	}
}

func TestNormalizeURLExtractsFromShareText(t *testing.T) {
	got, err := NormalizeURL("复制打开抖音，看看这个视频 https://v.douyin.com/iAbCdEf/ ，值得一看")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://v.douyin.com/iAbCdEf/" {
		t.Fatalf("unexpected url: %q", got)
	}
}

func TestNormalizeURLExtractsWhenShareTextStartsWithURL(t *testing.T) {
	got, err := NormalizeURL("https://v.douyin.com/iAbCdEf/ 复制此链接，打开抖音搜索")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://v.douyin.com/iAbCdEf/" {
		t.Fatalf("unexpected url: %q", got)
	}
}

func TestExtractURLsReturnsUniqueNormalizedURLs(t *testing.T) {
	got := ExtractURLs(`
		技术文章 https://example.com/a?x=1，
		重复 https://example.com/a?x=1
		视频：https://www.bilibili.com/video/BV1xx411c7mD/。
	`)
	want := []string{
		"https://example.com/a?x=1",
		"https://www.bilibili.com/video/BV1xx411c7mD/",
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractURLs() length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractURLs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDetectContentType(t *testing.T) {
	cases := map[string]task.ContentType{
		"https://www.bilibili.com/video/BV1xx411c7mD/": task.ContentVideo,
		"https://b23.tv/abc123":                        task.ContentVideo,
		"https://v.douyin.com/iAbCdEf/":                task.ContentVideo,
		"https://www.douyin.com/video/123":             task.ContentVideo,
		"https://example.com/article":                  task.ContentWebPage,
	}

	for raw, want := range cases {
		if got := DetectContentType(raw); got != want {
			t.Fatalf("DetectContentType(%q) = %s, want %s", raw, got, want)
		}
	}
}
