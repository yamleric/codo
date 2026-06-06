package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"

	"github.com/codo/codo/internal/application/pipeline"
	"github.com/codo/codo/internal/domain/task"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}
var sanitizer = bluemonday.StrictPolicy()

// HTTPFetcher implements pipeline.Fetcher using plain HTTP + readability.
type HTTPFetcher struct{}

func NewHTTP() *HTTPFetcher { return &HTTPFetcher{} }

func (f *HTTPFetcher) Fetch(ctx context.Context, t *task.Task) (string, error) {
	if t.URL == "" {
		if t.RawContent != "" {
			return t.RawContent, nil
		}
		return "", fmt.Errorf("fetcher: no URL or raw content")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return "", fmt.Errorf("fetcher: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Codo/1.0)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetcher: http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetcher: http %d for %s", resp.StatusCode, t.URL)
	}

	pageURL, _ := url.Parse(t.URL)
	parser := readability.NewParser()
	article, err := parser.Parse(resp.Body, pageURL)
	if err != nil {
		return "", fmt.Errorf("fetcher: readability: %w", err)
	}

	var buf bytes.Buffer
	if err := article.RenderText(&buf); err != nil {
		return "", fmt.Errorf("fetcher: render text: %w", err)
	}

	text := sanitizer.Sanitize(buf.String())
	text = strings.TrimSpace(text)
	if len([]rune(text)) < 100 {
		return "", fmt.Errorf("fetcher: content too short (%d chars)", len([]rune(text)))
	}
	return text, nil
}

// Verify at compile time.
var _ pipeline.Fetcher = (*HTTPFetcher)(nil)
