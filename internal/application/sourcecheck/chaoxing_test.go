package sourcecheck

import (
	"strings"
	"testing"
	"time"

	"github.com/codo/codo/internal/infra/store"
)

func TestIsActionableStatus(t *testing.T) {
	if !isActionableStatus("未提交") || !isActionableStatus("待做") {
		t.Fatal("unfinished statuses should be actionable")
	}
	if isActionableStatus("已完成") || isActionableStatus("已提交") {
		t.Fatal("finished statuses should not be actionable")
	}
}

func TestBuildChaoxingDueMessage(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.Local)
	due := now.Add(2 * time.Hour)
	msg := buildChaoxingMessage("due", store.SourceItemRow{
		ItemType: "homework",
		Course:   "软件工程",
		Title:    "实验报告",
		Status:   "未提交",
		DueAt:    &due,
	}, 24, func() time.Time { return now })
	for _, part := range []string{"学习通临近截止提醒", "软件工程", "实验报告", "剩余2小时"} {
		if !strings.Contains(msg, part) {
			t.Fatalf("message missing %q: %s", part, msg)
		}
	}
}
