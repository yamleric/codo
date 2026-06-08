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
