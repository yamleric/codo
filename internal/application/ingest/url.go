package ingest

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/codo/codo/internal/domain/task"
)

var pastedURL = regexp.MustCompile(`https?://[^\s<>"']+`)

// NormalizeURL accepts either a bare URL or a copied share sentence and returns
// the first valid http(s) URL.
func NormalizeURL(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}

	urls := ExtractURLs(raw)
	if len(urls) == 0 {
		return "", fmt.Errorf("missing http(s) url")
	}
	return urls[0], nil
}

func ExtractURLs(input string) []string {
	matches := pastedURL.FindAllString(input, -1)
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, candidate := range matches {
		candidate = trimURLPunctuation(candidate)
		parsed, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}
		if parsed.Hostname() == "" {
			continue
		}
		normalized := parsed.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func DetectContentType(rawURL string) task.ContentType {
	if IsVideoURL(rawURL) {
		return task.ContentVideo
	}
	return task.ContentWebPage
}

func IsVideoURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return isBilibiliHost(host) || isDouyinHost(host)
}

func trimURLPunctuation(raw string) string {
	return strings.TrimRight(raw, " \t\r\n.,;:!?，。；：！？)]}）】》>")
}

func isBilibiliHost(host string) bool {
	return host == "b23.tv" ||
		host == "bili2233.cn" ||
		host == "bilibili.com" ||
		strings.HasSuffix(host, ".bilibili.com")
}

func isDouyinHost(host string) bool {
	return host == "douyin.com" ||
		strings.HasSuffix(host, ".douyin.com") ||
		host == "iesdouyin.com" ||
		strings.HasSuffix(host, ".iesdouyin.com")
}
