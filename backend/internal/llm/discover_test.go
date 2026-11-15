package llm

import (
	"reflect"
	"testing"
)

func TestRankGemini(t *testing.T) {
	ids := []string{"gemini-2.5-flash", "gemini-3.8-flash", "gemini-3.8-flash-lite", "gemini-3.8-flash-preview-x",
		"gemini-2.5-flash-image", "gemini-2.5-pro", "text-embedding-004", "gemini-3.8-flash-live"}
	got := rankGemini(ids)
	want := []string{"gemini-3.8-flash", "gemini-3.8-flash-preview-x", "gemini-3.8-flash-lite", "gemini-2.5-flash"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRankOpenRouterAndGroq(t *testing.T) {
	or := rankOpenRouter([]string{"x/y", "qwen/qwen3:free", "meta-llama/llama-3.3-70b-instruct:free"})
	if len(or) != 2 || or[0] != "meta-llama/llama-3.3-70b-instruct:free" {
		t.Fatalf("openrouter: %v", or)
	}
	g := rankGroq([]string{"whisper-large-v3", "llama-3.1-8b-instant", "llama-3.3-70b-versatile"})
	if len(g) != 2 || g[0] != "llama-3.3-70b-versatile" {
		t.Fatalf("groq: %v", g)
	}
}
