package llm

import (
	"strings"
	"testing"
)

func TestFilterPromptIncludesPreferenceMemory(t *testing.T) {
	prompt := filterPrompt(UserPreferences{
		MemoryEnabled:    true,
		PreferenceMemory: "优先通知：AI 产品落地",
	})
	if !strings.Contains(prompt, "用户长期偏好记忆") {
		t.Fatalf("prompt missing preference memory section: %s", prompt)
	}
	if !strings.Contains(prompt, "AI 产品落地") {
		t.Fatalf("prompt missing memory content: %s", prompt)
	}
}

func TestNormalizePreferencesDisablesEmptyMemory(t *testing.T) {
	prefs := normalizePreferences(UserPreferences{MemoryEnabled: true, PreferenceMemory: "  "})
	if prefs.MemoryEnabled {
		t.Fatal("empty preference memory should disable memory prompt")
	}
}

func TestLooksMostlyEnglish(t *testing.T) {
	english := strings.Repeat("This article explains how retrieval augmented generation improves personal knowledge workflows. ", 4)
	if !looksMostlyEnglish(english) {
		t.Fatal("expected English text to be detected")
	}
	chinese := strings.Repeat("这是一篇中文资料，主要讨论个人知识库、收藏意图和内容总结。", 8)
	if looksMostlyEnglish(chinese) {
		t.Fatal("expected Chinese text not to be detected as English")
	}
}

func TestNormalizeTranslationPolicy(t *testing.T) {
	policy := normalizeTranslationPolicy(TranslationPolicy{Enabled: true, Scope: "knowledge", MaxChars: 50000})
	if !policy.Enabled || policy.Scope != "knowledge" || policy.MaxChars != 30000 || policy.Mode != "english_only" {
		t.Fatalf("unexpected policy: %#v", policy)
	}
}
