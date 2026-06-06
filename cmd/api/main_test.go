package main

import "testing"

func TestBookmarkInputsFromPayloadExtractsAndDeduplicatesURLs(t *testing.T) {
	title := "显式标题"
	folder := "产品"
	note := "稍后读"
	url := "https://example.com/article"
	inputs := bookmarkInputsFromPayload(bookmarkImportPayload{
		URL:    "分享文本 https://example.com/article ",
		Text:   "https://example.com/article\nhttps://example.com/second，",
		Folder: "待读",
		Bookmarks: []bookmarkPayload{
			{URL: &url, Title: &title, Folder: &folder, Note: &note},
		},
	})

	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2: %#v", len(inputs), inputs)
	}
	if inputs[0].URL != "https://example.com/article" || inputs[0].Folder != "待读" {
		t.Fatalf("unexpected first input: %#v", inputs[0])
	}
	if inputs[1].URL != "https://example.com/second" || inputs[1].Folder != "待读" {
		t.Fatalf("unexpected second input: %#v", inputs[1])
	}
}

func TestBookmarkInputsFromPayloadKeepsExplicitMetadata(t *testing.T) {
	title := "文章"
	folder := "技术"
	note := "重点"
	url := "https://example.com/a"
	inputs := bookmarkInputsFromPayload(bookmarkImportPayload{
		Bookmarks: []bookmarkPayload{
			{URL: &url, Title: &title, Folder: &folder, Note: &note},
		},
	})

	if len(inputs) != 1 {
		t.Fatalf("len(inputs) = %d, want 1", len(inputs))
	}
	if inputs[0].Title != title || inputs[0].Folder != folder || inputs[0].Note != note {
		t.Fatalf("metadata not preserved: %#v", inputs[0])
	}
}
