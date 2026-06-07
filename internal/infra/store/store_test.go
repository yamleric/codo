package store

import "testing"

func TestDailyReportFromRawPolicyUsesDefaultsWhenMissing(t *testing.T) {
	report := dailyReportFromRawPolicy(`{"summary_style":"concise"}`)
	if report.Hour != 21 || report.Timezone != "Asia/Shanghai" || report.MaxItems != 20 || report.Enabled {
		t.Fatalf("unexpected defaults: %#v", report)
	}
}

func TestDailyReportFromRawPolicyKeepsExplicitMidnightHour(t *testing.T) {
	report := dailyReportFromRawPolicy(`{"daily_report":{"enabled":true,"email":"me@example.com","hour":0,"timezone":"Asia/Shanghai","max_items":10}}`)
	report = NormalizeDailyReport(report)
	if !report.Enabled || report.Hour != 0 || report.Email != "me@example.com" || report.MaxItems != 10 {
		t.Fatalf("unexpected explicit report config: %#v", report)
	}
}
