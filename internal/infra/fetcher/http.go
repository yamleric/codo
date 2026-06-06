package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/playwright-community/playwright-go"

	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}
var sanitizer = bluemonday.StrictPolicy()
var whitespace = regexp.MustCompile(`\s+`)

const (
	wechatParserReference   = "https://github.com/huanjuedadehen/wechat-article-parser"
	defaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"
)

// HTTPFetcher implements pipeline.Fetcher using plain HTTP + readability.
type HTTPFetcher struct {
	browserExecutable string
	browserTimeout    time.Duration
}

func NewHTTP() *HTTPFetcher {
	return &HTTPFetcher{
		browserExecutable: getenvDefault("PLAYWRIGHT_CHROMIUM_EXECUTABLE", defaultChromiumExecutable()),
		browserTimeout:    time.Duration(getenvInt("PLAYWRIGHT_TIMEOUT_SECONDS", 45)) * time.Second,
	}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, t *task.Task) (string, error) {
	if t.URL == "" {
		if t.RawContent() != "" {
			return t.RawContent(), nil
		}
		return "", fmt.Errorf("fetcher: no URL or raw content")
	}

	pageURL, _ := url.Parse(t.URL)
	if isZhihuHost(pageURL.Hostname()) {
		return f.fetchZhihuWithPlaywright(ctx, t.URL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return "", fmt.Errorf("fetcher: build request: %w", err)
	}
	req.Header.Set("User-Agent", defaultBrowserUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetcher: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetcher: http %d for %s", resp.StatusCode, t.URL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fetcher: read body: %w", err)
	}

	if pageURL.Hostname() == "mp.weixin.qq.com" {
		text, err := extractWechatArticle(body)
		if err != nil {
			return "", fmt.Errorf("fetcher: wechat article: %w", err)
		}
		return validateContent(text)
	}

	parser := readability.NewParser()
	article, err := parser.Parse(bytes.NewReader(body), pageURL)
	if err != nil {
		return "", fmt.Errorf("fetcher: readability: %w", err)
	}

	var buf bytes.Buffer
	if err := article.RenderText(&buf); err != nil {
		return "", fmt.Errorf("fetcher: render text: %w", err)
	}

	return validateContent(buf.String())
}

func (f *HTTPFetcher) fetchZhihuWithPlaywright(ctx context.Context, rawURL string) (string, error) {
	if f.browserExecutable == "" {
		return "", fmt.Errorf("fetcher: playwright chromium executable not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, f.browserTimeout)
	defer cancel()

	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := f.renderZhihu(rawURL)
		done <- result{text: text, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("fetcher: playwright timeout: %w", ctx.Err())
	case got := <-done:
		if got.err != nil {
			return "", got.err
		}
		return validateContent(got.text)
	}
}

func (f *HTTPFetcher) renderZhihu(rawURL string) (string, error) {
	pw, err := playwright.Run(&playwright.RunOptions{SkipInstallBrowsers: true})
	if err != nil {
		return "", fmt.Errorf("fetcher: start playwright: %w", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		ExecutablePath: playwright.String(f.browserExecutable),
		Headless:       playwright.Bool(true),
		Args: []string{
			"--no-sandbox",
			"--disable-dev-shm-usage",
			"--disable-gpu",
		},
	})
	if err != nil {
		return "", fmt.Errorf("fetcher: launch chromium: %w", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: playwright.String(defaultBrowserUserAgent),
		Locale:    playwright.String("zh-CN"),
		Viewport:  &playwright.Size{Width: 1366, Height: 900},
		ExtraHttpHeaders: map[string]string{
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		},
	})
	if err != nil {
		return "", fmt.Errorf("fetcher: create page: %w", err)
	}
	defer page.Close()
	page.SetDefaultTimeout(float64(f.browserTimeout / time.Millisecond))

	if _, err := page.Goto(rawURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(float64(f.browserTimeout / time.Millisecond)),
	}); err != nil {
		return "", fmt.Errorf("fetcher: goto zhihu: %w", err)
	}

	_ = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10_000),
	})
	tryExpandZhihuContent(page)

	text := extractZhihuRenderedText(page)
	if text == "" {
		return "", fmt.Errorf("fetcher: zhihu content not found")
	}
	return text, nil
}

func extractZhihuRenderedText(page playwright.Page) string {
	titleBlocks := collectRenderedTexts(page, []string{
		".QuestionHeader-title",
		".Post-Title",
		".ContentItem-title",
	})
	contentBlocks := collectRenderedTexts(page, []string{
		".Post-RichTextContainer",
		".RichContent-inner",
		".ztext",
	})

	fallback := ""
	if body, err := page.Locator("body").InnerText(); err == nil {
		fallback = body
	}
	return composeZhihuText(titleBlocks, contentBlocks, fallback)
}

func tryExpandZhihuContent(page playwright.Page) {
	for _, selector := range []string{
		"button.ContentItem-more",
		"button:has-text(\"阅读全文\")",
		"button:has-text(\"展开阅读全文\")",
		"button:has-text(\"展开全部\")",
		".RichContent-collapsedText",
	} {
		locator := page.Locator(selector)
		count, err := locator.Count()
		if err != nil {
			continue
		}
		if count > 3 {
			count = 3
		}
		for i := 0; i < count; i++ {
			_ = locator.Nth(i).Click(playwright.LocatorClickOptions{
				Timeout: playwright.Float(1_500),
			})
		}
	}
}

func collectRenderedTexts(page playwright.Page, selectors []string) []string {
	blocks := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		texts, err := page.Locator(selector).AllInnerTexts()
		if err != nil {
			continue
		}
		blocks = append(blocks, texts...)
	}
	return blocks
}

func composeZhihuText(titleBlocks, contentBlocks []string, fallback string) string {
	text := normalizeTextBlocks(append(titleBlocks, contentBlocks...))
	fallback = cleanText(fallback)
	if len([]rune(text)) < 100 && len([]rune(fallback)) > len([]rune(text)) {
		return fallback
	}
	return text
}

func extractWechatArticle(body []byte) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	// Verification detection and alternate article containers are informed by
	// wechat-article-parser (MIT); this implementation remains native Go.
	// See CREDITS.md and wechatParserReference.
	if doc.Find(`script[src*="secitptpage/template/verify.js"]`).Length() > 0 {
		return "", fmt.Errorf("verification required")
	}

	content := doc.Find("#js_content, .rich_media_content, .original_page").First()
	if content.Length() == 0 {
		return "", fmt.Errorf("missing article content")
	}
	content.Find("script, style, noscript").Remove()

	title := cleanText(doc.Find("#activity-name").First().Text())
	account := cleanText(doc.Find("#js_name").First().Text())
	paragraphs := make([]string, 0, 32)
	content.Find("p, li, blockquote, h1, h2, h3, h4, h5, h6").Each(func(_ int, selection *goquery.Selection) {
		text := cleanText(selection.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})
	if len(paragraphs) == 0 {
		paragraphs = append(paragraphs, cleanText(content.Text()))
	}

	parts := make([]string, 0, len(paragraphs)+2)
	if title != "" {
		parts = append(parts, "标题："+title)
	}
	if account != "" {
		parts = append(parts, "来源："+account)
	}
	parts = append(parts, paragraphs...)
	return strings.Join(parts, "\n\n"), nil
}

func validateContent(text string) (string, error) {
	text = sanitizer.Sanitize(text)
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 100 {
		return "", fmt.Errorf("fetcher: content too short (%d chars)", len([]rune(text)))
	}
	return text, nil
}

func cleanText(text string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(text, " "))
}

func normalizeTextBlocks(blocks []string) string {
	out := make([]string, 0, len(blocks))
	seen := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		text := cleanText(block)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		out = append(out, text)
		seen[text] = struct{}{}
	}
	return strings.Join(out, "\n\n")
}

func isZhihuHost(host string) bool {
	host = strings.ToLower(host)
	return host == "zhihu.com" || strings.HasSuffix(host, ".zhihu.com")
}

func defaultChromiumExecutable() string {
	for _, candidate := range []string{"/usr/bin/chromium-browser", "/usr/bin/chromium", "/usr/bin/google-chrome"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath("chromium-browser"); err == nil {
		return path
	}
	if path, err := exec.LookPath("chromium"); err == nil {
		return path
	}
	if path, err := exec.LookPath("google-chrome"); err == nil {
		return path
	}
	return ""
}

// Verify at compile time.
var _ pipeline.Fetcher = (*HTTPFetcher)(nil)
