package pipeline

import "testing"

func TestEmailImportantExtractionEnabled(t *testing.T) {
	if !emailImportantExtractionEnabled("主题：测试\n\n正文") {
		t.Fatal("default should enable important extraction")
	}
	if emailImportantExtractionEnabled("重要邮件单独提取：false\n主题：测试\n\n正文") {
		t.Fatal("explicit false should disable important extraction")
	}
}
