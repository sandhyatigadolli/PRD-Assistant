package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Model names change often (and free tiers differ per model), so instead of
// hard-coding one, we ask the provider which models this key can see and try
// the best few until one answers.

func listModels(ctx context.Context, base, key string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("models list: status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, strings.TrimPrefix(m.ID, "models/"))
	}
	return ids, nil
}

// Candidates returns models to try, best first. The provider default is always
// included last as a fallback. If the key is rejected the list call fails and
// only the fallback is returned (Verify then reports the real error).
func Candidates(ctx context.Context, c Config) []string {
	d, ok := providerDefaults[c.Provider]
	if !ok {
		return nil
	}
	var ranked []string
	if ids, err := listModels(ctx, d.base, strings.TrimSpace(c.APIKey)); err == nil {
		switch c.Provider {
		case "gemini":
			ranked = rankGemini(ids)
		case "groq":
			ranked = rankGroq(ids)
		case "openrouter":
			ranked = rankOpenRouter(ids)
		case "openai":
			ranked = rankOpenAI(ids)
		}
	}
	if len(ranked) > 4 {
		ranked = ranked[:4]
	}
	for _, r := range ranked {
		if r == d.model {
			return ranked
		}
	}
	return append(ranked, d.model)
}

var geminiVer = regexp.MustCompile(`^gemini-(\d+(?:\.\d+)?)-flash`)

func rankGemini(ids []string) []string {
	type m struct {
		id         string
		ver        float64
		lite, prev bool
	}
	var ms []m
	for _, id := range ids {
		sub := geminiVer.FindStringSubmatch(id)
		if sub == nil || containsAny(id, "image", "tts", "audio", "live", "embedding", "vision", "exp", "native", "thinking", "robotics", "computer") {
			continue
		}
		v, _ := strconv.ParseFloat(sub[1], 64)
		ms = append(ms, m{id, v, strings.Contains(id, "lite"), strings.Contains(id, "preview")})
	}
	sort.SliceStable(ms, func(i, j int) bool {
		a, b := ms[i], ms[j]
		if a.ver != b.ver {
			return a.ver > b.ver
		}
		if a.lite != b.lite {
			return !a.lite
		}
		if a.prev != b.prev {
			return !a.prev
		}
		return len(a.id) < len(b.id)
	})
	out := make([]string, len(ms))
	for i, x := range ms {
		out[i] = x.id
	}
	return out
}

func rankGroq(ids []string) []string {
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	var out []string
	for _, pref := range []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "openai/gpt-oss-20b", "openai/gpt-oss-120b"} {
		if have[pref] {
			out = append(out, pref)
		}
	}
	for _, id := range ids {
		if !containsAny(id, "whisper", "tts", "guard", "playai", "orpheus", "embed") && !contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func rankOpenRouter(ids []string) []string {
	var free []string
	for _, id := range ids {
		if strings.HasSuffix(id, ":free") {
			free = append(free, id)
		}
	}
	score := func(id string) int {
		for i, k := range []string{"llama-3.3", "gemini", "deepseek", "qwen", "gpt-oss", "mistral", "llama"} {
			if strings.Contains(id, k) {
				return i
			}
		}
		return 99
	}
	sort.SliceStable(free, func(i, j int) bool { return score(free[i]) < score(free[j]) })
	return free
}

func rankOpenAI(ids []string) []string {
	var out []string
	for _, pref := range []string{"gpt-4o-mini", "gpt-4.1-mini", "gpt-5-mini"} {
		if contains(ids, pref) {
			out = append(out, pref)
		}
	}
	return out
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
