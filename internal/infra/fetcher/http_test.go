package fetcher

import (
	"strings"
	"testing"
)

func TestExtractWechatArticle(t *testing.T) {
	body := []byte(`
		<html><body>
			<h1 id="activity-name"><span>测试文章标题</span></h1>
			<a id="js_name">测试公众号</a>
			<div id="js_content">
				<p>第一段正文，包含需要保留的内容。</p>
				<section><p>第二段正文，验证段落之间有换行。</p></section>
				<script>should not appear</script>
			</div>
		</body></html>`)

	text, err := extractWechatArticle(body)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"标题：测试文章标题", "来源：测试公众号", "第一段正文", "第二段正文"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in extracted text: %s", expected, text)
		}
	}
	if strings.Contains(text, "should not appear") {
		t.Fatalf("script content leaked into extracted text: %s", text)
	}
}

func TestExtractWechatArticleRequiresContentNode(t *testing.T) {
	_, err := extractWechatArticle([]byte(`<html><body><p>not an article</p></body></html>`))
	if err == nil {
		t.Fatal("expected missing content error")
	}
}

func TestExtractWechatArticleDetectsVerificationPage(t *testing.T) {
	body := []byte(`<html><body><script src="/secitptpage/template/verify.js"></script></body></html>`)
	_, err := extractWechatArticle(body)
	if err == nil || !strings.Contains(err.Error(), "verification required") {
		t.Fatalf("expected verification error, got %v", err)
	}
}

func TestExtractWechatArticleSupportsAlternateContentContainer(t *testing.T) {
	body := []byte(`<html><body><div class="rich_media_content"><p>备用正文容器中的有效文章内容。</p></div></body></html>`)
	text, err := extractWechatArticle(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "备用正文容器") {
		t.Fatalf("unexpected extracted text: %s", text)
	}
}

func TestIsZhihuHost(t *testing.T) {
	cases := map[string]bool{
		"zhihu.com":          true,
		"www.zhihu.com":      true,
		"zhuanlan.zhihu.com": true,
		"notzhihu.com":       false,
		"zhihu.com.example":  false,
		"www.bilibili.com":   false,
	}

	for host, want := range cases {
		if got := isZhihuHost(host); got != want {
			t.Fatalf("isZhihuHost(%q) = %t, want %t", host, got, want)
		}
	}
}

func TestNormalizeTextBlocks(t *testing.T) {
	got := normalizeTextBlocks([]string{
		"  标题  ",
		"标题",
		"\n正文   第一段\n",
		"",
		"正文 第二段",
	})
	want := "标题\n\n正文 第一段\n\n正文 第二段"
	if got != want {
		t.Fatalf("normalizeTextBlocks() = %q, want %q", got, want)
	}
}

func TestComposeZhihuTextCombinesTitleAndContent(t *testing.T) {
	got := composeZhihuText(
		[]string{"知乎问题标题"},
		[]string{"回答正文第一段", "回答正文第二段", "知乎问题标题"},
		"",
	)
	want := "知乎问题标题\n\n回答正文第一段\n\n回答正文第二段"
	if got != want {
		t.Fatalf("composeZhihuText() = %q, want %q", got, want)
	}
}

func TestComposeZhihuTextFallsBackToLongerBody(t *testing.T) {
	body := strings.Repeat("正文", 80)
	got := composeZhihuText([]string{"短标题"}, nil, body)
	if got != body {
		t.Fatalf("composeZhihuText() should use longer body fallback")
	}
}

func TestValidateZhihuRenderedContentRejectsRestrictionPage(t *testing.T) {
	text := "当前请求存在异常，暂时无法访问。错误码 40362。如有疑问请联系知乎小管家。"
	if _, err := validateZhihuRenderedContent(text); err == nil {
		t.Fatal("expected zhihu restriction page to be rejected")
	}
}
