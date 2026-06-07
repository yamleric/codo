package sources

import (
	"testing"
	"time"
)

func TestParseChaoxingDueFromRemainingTime(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.Local)
	due := parseChaoxingDue(now, "作业状态未提交 剩余1天2小时30分钟")
	if due == nil {
		t.Fatal("due is nil")
	}
	want := now.Add(26*time.Hour + 30*time.Minute)
	if !due.Equal(want) {
		t.Fatalf("due = %s, want %s", due, want)
	}
}

func TestExternalIDFromURL(t *testing.T) {
	id := externalIDFromURL("https://mooc1.chaoxing.com/work?courseId=1&taskrefId=abc123", []string{"taskrefId"})
	if id != "abc123" {
		t.Fatalf("id = %q", id)
	}
}
