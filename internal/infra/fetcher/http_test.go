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
