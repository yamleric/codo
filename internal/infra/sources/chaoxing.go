package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type ChaoxingCredentials struct {
	Account  string
	Password string
	Cookie   string
}

type ChaoxingItem struct {
	Type          string
	ExternalID    string
	Course        string
	Title         string
	Status        string
	URL           string
	DueAt         *time.Time
	RemainingText string
	RawText       string
}

type ChaoxingClient struct {
	httpClient *http.Client
	now        func() time.Time
}

func NewChaoxingClient() *ChaoxingClient {
	jar, _ := cookiejar.New(nil)
	return &ChaoxingClient{
		httpClient: &http.Client{Timeout: 25 * time.Second, Jar: jar},
		now:        time.Now,
	}
}

func (c *ChaoxingClient) FetchItems(ctx context.Context, creds ChaoxingCredentials) ([]ChaoxingItem, error) {
	if strings.TrimSpace(creds.Account) == "" && strings.TrimSpace(creds.Cookie) == "" {
		return nil, fmt.Errorf("chaoxing: account or cookie is required")
	}
	if strings.TrimSpace(creds.Cookie) != "" {
		c.applyCookie(creds.Cookie)
	} else {
		if err := c.login(ctx, creds.Account, creds.Password); err != nil {
			return nil, err
		}
	}

	homeworks, err := c.fetchHomeworks(ctx)
	if err != nil && strings.TrimSpace(creds.Cookie) != "" && strings.TrimSpace(creds.Password) != "" {
		if loginErr := c.login(ctx, creds.Account, creds.Password); loginErr == nil {
			homeworks, err = c.fetchHomeworks(ctx)
		}
	}
	if err != nil {
		return nil, err
	}
	exams, err := c.fetchExams(ctx)
	if err != nil {
		return nil, err
	}
	return append(homeworks, exams...), nil
}

func (c *ChaoxingClient) login(ctx context.Context, account, password string) error {
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return fmt.Errorf("chaoxing: account and password are required")
	}
	endpoint := "https://passport2-api.chaoxing.com/v11/loginregister"
	values := url.Values{}
	values.Set("code", password)
	values.Set("cx_xxt_passport", "json")
	values.Set("uname", account)
	values.Set("loginType", "1")
	values.Set("roleSelect", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return err
	}
	chaoxingHeaders(req)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("chaoxing login: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("chaoxing login: http %d", res.StatusCode)
	}
	var payload struct {
		Status bool   `json:"status"`
		Msg    string `json:"msg"`
		Mes    string `json:"mes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return fmt.Errorf("chaoxing login response: %w", err)
	}
	if !payload.Status && payload.Mes != "验证通过" {
		msg := strings.TrimSpace(payload.Msg)
		if msg == "" {
			msg = strings.TrimSpace(payload.Mes)
		}
		if msg == "" {
			msg = "login failed"
		}
		return fmt.Errorf("chaoxing login: %s", msg)
	}
	return nil
}

func (c *ChaoxingClient) fetchHomeworks(ctx context.Context) ([]ChaoxingItem, error) {
	doc, err := c.getDocument(ctx, "https://mooc1-api.chaoxing.com/work/stu-work")
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title != "" && title != "作业列表" && strings.Contains(title, "登录") {
		return nil, fmt.Errorf("chaoxing homework: login required")
	}

	var items []ChaoxingItem
	doc.Find("li").Each(func(_ int, li *goquery.Selection) {
		raw := compactText(li.Text())
		dataURL, _ := li.Attr("data")
		if raw == "" && dataURL == "" {
			return
		}
		item := ChaoxingItem{
			Type:          "homework",
			URL:           normalizeChaoxingURL(dataURL),
			RawText:       raw,
			RemainingText: extractRemainingText(raw),
		}
		if name := compactText(li.Find("p").First().Text()); name != "" {
			item.Title = name
		}
		if item.Title == "" {
			item.Title = extractBetween(raw, "作业名称", "作业状态")
		}
		if item.Status == "" {
			item.Status = statusFromSpans(li, []string{"未提交", "未交", "已提交", "已完成", "待批阅", "已过期"})
		}
		if item.Status == "" {
			item.Status = extractBetween(raw, "作业状态", "所属课程")
		}
		item.Course = courseFromHomework(li, raw)
		item.DueAt = parseChaoxingDue(c.now(), raw)
		item.ExternalID = externalIDFromURL(item.URL, []string{"taskrefId", "workId", "answerId"})
		if item.ExternalID == "" {
			item.ExternalID = stableItemID(item.Type, item.Course, item.Title, item.URL)
		}
		if item.Title != "" || item.Status != "" {
			items = append(items, item)
		}
	})
	return items, nil
}

func (c *ChaoxingClient) fetchExams(ctx context.Context) ([]ChaoxingItem, error) {
	doc, err := c.getDocument(ctx, "https://mooc1-api.chaoxing.com/exam/phone/examcode")
	if err != nil {
		return nil, err
	}
	var items []ChaoxingItem
	doc.Find("li").Each(func(_ int, li *goquery.Selection) {
		raw := compactText(li.Text())
		dataURL, _ := li.Attr("data")
		if raw == "" && dataURL == "" {
			return
		}
		item := ChaoxingItem{
			Type:          "exam",
			URL:           normalizeChaoxingURL(dataURL),
			RawText:       raw,
			RemainingText: extractRemainingText(raw),
		}
		item.Title = compactText(li.Find("dl dt").First().Text())
		if item.Title == "" {
			item.Title = compactText(li.Find("p").First().Text())
		}
		item.Status = statusFromSpans(li, []string{"未开始", "待做", "未交", "已完成", "已过期"})
		if item.Status == "" {
			item.Status = extractKnownStatus(raw, []string{"未开始", "待做", "未交", "已完成", "已过期"})
		}
		item.Course = compactText(li.Find("dd").First().Text())
		item.DueAt = parseChaoxingDue(c.now(), raw)
		item.ExternalID = externalIDFromURL(item.URL, []string{"examRelationId", "paperId", "id"})
		if item.ExternalID == "" {
			item.ExternalID = stableItemID(item.Type, item.Course, item.Title, item.URL)
		}
		if item.Title != "" || item.Status != "" {
			items = append(items, item)
		}
	})
	return items, nil
}

func (c *ChaoxingClient) getDocument(ctx context.Context, target string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	chaoxingHeaders(req)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chaoxing fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("chaoxing fetch: http %d", res.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, fmt.Errorf("chaoxing parse html: %w", err)
	}
	return doc, nil
}

func (c *ChaoxingClient) applyCookie(cookieHeader string) {
	var cookies []*http.Cookie
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		cookies = append(cookies, &http.Cookie{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	if len(cookies) > 0 && c.httpClient.Jar != nil {
		for _, host := range []string{
			"https://chaoxing.com",
			"https://mooc1-api.chaoxing.com",
			"https://mooc1.chaoxing.com",
			"https://passport2-api.chaoxing.com",
			"https://passport2.chaoxing.com",
		} {
			u, _ := url.Parse(host)
			c.httpClient.Jar.SetCookies(u, cookies)
		}
	}
}

func chaoxingHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.6")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.8,*/*;q=0.7")
}

func compactText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.Join(strings.Fields(value), " ")
}

func normalizeChaoxingURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	if strings.HasPrefix(value, "/") {
		return "https://mooc1-api.chaoxing.com" + value
	}
	return value
}

func statusFromSpans(li *goquery.Selection, statuses []string) string {
	var status string
	li.Find("span,p.status").EachWithBreak(func(_ int, node *goquery.Selection) bool {
		text := compactText(node.Text())
		if text == "" {
			return true
		}
		if known := extractKnownStatus(text, statuses); known != "" {
			status = known
			return false
		}
		return true
	})
	return status
}

func courseFromHomework(li *goquery.Selection, raw string) string {
	if course := extractBetween(raw, "所属课程", "剩余时间"); course != "" {
		return course
	}
	var values []string
	li.Find("span").Each(func(_ int, span *goquery.Selection) {
		text := compactText(span.Text())
		if text != "" && extractKnownStatus(text, []string{"未提交", "未交", "已提交", "已完成", "待批阅", "已过期"}) == "" && !strings.Contains(text, "剩余") {
			values = append(values, text)
		}
	})
	if len(values) > 0 {
		return values[len(values)-1]
	}
	return ""
}

func extractKnownStatus(text string, statuses []string) string {
	for _, status := range statuses {
		if strings.Contains(text, status) {
			return status
		}
	}
	return ""
}

func extractBetween(text, start, end string) string {
	startIdx := strings.Index(text, start)
	if startIdx < 0 {
		return ""
	}
	value := text[startIdx+len(start):]
	if endIdx := strings.Index(value, end); endIdx >= 0 {
		value = value[:endIdx]
	}
	return strings.TrimSpace(value)
}

func extractRemainingText(text string) string {
	if idx := strings.Index(text, "剩余"); idx >= 0 {
		return strings.TrimSpace(text[idx:])
	}
	return ""
}

func parseChaoxingDue(now time.Time, text string) *time.Time {
	if due := parseRelativeDue(now, text); due != nil {
		return due
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`20\d{2}[-/]\d{1,2}[-/]\d{1,2}\s+\d{1,2}:\d{2}`),
		regexp.MustCompile(`20\d{2}[-/]\d{1,2}[-/]\d{1,2}`),
	}
	for _, pattern := range patterns {
		match := pattern.FindString(text)
		if match == "" {
			continue
		}
		match = strings.ReplaceAll(match, "/", "-")
		for _, layout := range []string{"2006-01-02 15:04", "2006-1-2 15:04", "2006-01-02", "2006-1-2"} {
			if t, err := time.ParseInLocation(layout, match, time.Local); err == nil {
				return &t
			}
		}
	}
	return nil
}

func parseRelativeDue(now time.Time, text string) *time.Time {
	if !strings.Contains(text, "剩余") {
		return nil
	}
	days := firstInt(regexp.MustCompile(`(\d+)\s*天`).FindStringSubmatch(text))
	hours := firstInt(regexp.MustCompile(`(\d+)\s*小时`).FindStringSubmatch(text))
	minutes := firstInt(regexp.MustCompile(`(\d+)\s*分钟`).FindStringSubmatch(text))
	if days == 0 && hours == 0 && minutes == 0 {
		return nil
	}
	due := now.Add(time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute)
	return &due
}

func firstInt(match []string) int {
	if len(match) < 2 {
		return 0
	}
	var value int
	_, _ = fmt.Sscanf(match[1], "%d", &value)
	return value
}

func externalIDFromURL(raw string, keys []string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	values := parsed.Query()
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func stableItemID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}
