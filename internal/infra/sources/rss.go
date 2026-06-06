package sources

import (
	"context"
	"fmt"
	"time"

	"github.com/mmcdole/gofeed"
)

// RSSItem is a single item fetched from an RSS/Atom/JSON feed.
type RSSItem struct {
	URL         string
	Title       string
	PublishedAt time.Time
}

// FetchRSS fetches up to maxItems items from the given feed URL published after since.
func FetchRSS(ctx context.Context, feedURL string, since time.Time, maxItems int) ([]RSSItem, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("rss fetch %s: %w", feedURL, err)
	}

	var items []RSSItem
	for _, item := range feed.Items {
		if len(items) >= maxItems {
			break
		}
		pub := time.Now()
		if item.PublishedParsed != nil {
			pub = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			pub = *item.UpdatedParsed
		}
		if !since.IsZero() && !pub.After(since) {
			continue
		}
		link := item.Link
		if link == "" {
			continue
		}
		items = append(items, RSSItem{URL: link, Title: item.Title, PublishedAt: pub})
	}
	return items, nil
}
