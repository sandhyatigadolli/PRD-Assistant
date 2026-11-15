// Package llm contains the LLM API clients. Three wire formats cover everything:
//   - OpenAI-compatible /chat/completions (OpenAI, Gemini, Ollama, LM Studio)
//   - Anthropic /v1/messages
//
// The API key lives only in memory (Holder); it is never written to disk or DB.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

type Client interface {
	Chat(ctx context.Context, system string, msgs []Message) (string, error)
	Describe() Info
}

type Info struct {
	Mode     string `json:"mode"` // api | local
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
}

var httpClient = &http.Client{Timeout: 90 * time.Second}

// ---------- OpenAI-compatible ----------

type openAIClient struct {
	info   Info
	base   string
	apiKey string
}

func (c *openAIClient) Describe() Info { return c.info }

func (c *openAIClient) Chat(ctx context.Context, system string, msgs []Message) (string, error) {
	all := make([]Message, 0, len(msgs)+1)
	if system != "" {
		all = append(all, Message{"system", system})
	}
	all = append(all, msgs...)
	body, _ := json.Marshal(map[string]any{"model": c.info.Model, "messages": all})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.base+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	raw, err := do(req)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("unexpected LLM response: %.200s", raw)
	}
	return out.Choices[0].Message.Content, nil
}

// ---------- Anthropic ----------

type anthropicClient struct {
	info   Info
	apiKey string
}

func (c *anthropicClient) Describe() Info { return c.info }

func (c *anthropicClient) Chat(ctx context.Context, system string, msgs []Message) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": c.info.Model, "max_tokens": 4096, "system": system, "messages": msgs,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	raw, err := do(req)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("unexpected LLM response: %.200s", raw)
	}
	var sb strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("empty LLM response: %.200s", raw)
	}
	return sb.String(), nil
}

func do(req *http.Request) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach LLM: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errors.New("the API key was rejected (401/403). Check the key and provider")
	}
	if resp.StatusCode == 429 {
		return nil, errors.New("rate limit or quota exceeded (429). Check your plan/credits, or wait a minute")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LLM error %d: %.300s", resp.StatusCode, raw)
	}
	return raw, nil
}

// ---------- Factory ----------

var providerDefaults = map[string]struct{ base, model string }{
	"openai": {"https://api.openai.com/v1", "gpt-4o-mini"},
	// "gemini-flash-latest" is Google's alias that always points at the newest Flash model.
	"gemini":     {"https://generativelanguage.googleapis.com/v1beta/openai", "gemini-flash-latest"},
	"groq":       {"https://api.groq.com/openai/v1", "llama-3.3-70b-versatile"},
	"openrouter": {"https://openrouter.ai/api/v1", "meta-llama/llama-3.3-70b-instruct:free"},
}

const anthropicDefaultModel = "claude-haiku-5-5"

type Config struct {
	Mode     string `json:"mode"`     // api | local
	Provider string `json:"provider"` // gemini | groq | openrouter | openai | anthropic (api mode)
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"` // local mode: e.g. http://localhost:11434
}

func New(c Config) (Client, error) {
	switch c.Mode {
	case "api":
		if strings.TrimSpace(c.APIKey) == "" {
			return nil, errors.New("an API key is required")
		}
		key := strings.TrimSpace(c.APIKey)
		if c.Provider == "anthropic" {
			m := orDefault(c.Model, anthropicDefaultModel)
			return &anthropicClient{info: Info{Mode: "api", Provider: "anthropic", Model: m}, apiKey: key}, nil
		}
		d, ok := providerDefaults[c.Provider]
		if !ok {
			return nil, fmt.Errorf("unknown provider %q (use gemini, groq, openrouter, openai or anthropic)", c.Provider)
		}
		m := orDefault(c.Model, d.model)
		return &openAIClient{info: Info{Mode: "api", Provider: c.Provider, Model: m}, base: d.base, apiKey: key}, nil
	case "local":
		base := strings.TrimRight(orDefault(c.BaseURL, "http://localhost:11434"), "/")
		model := c.Model
		if model == "" {
			ms, err := LocalModels(context.Background(), base)
			if err != nil {
				return nil, err
			}
			model = ms[0]
		}
		return &openAIClient{info: Info{Mode: "local", Provider: "local", Model: model, BaseURL: base}, base: base + "/v1"}, nil
	}
	return nil, errors.New("mode must be 'api' or 'local'")
}

func orDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}

// LocalModels checks a locally running LLM server (Ollama, or any server that
// exposes /v1/models such as LM Studio) and returns the models it offers.
func LocalModels(ctx context.Context, base string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	get := func(path string) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", base+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("status %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	var models []string
	if raw, err := get("/api/tags"); err == nil { // Ollama
		var t struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if json.Unmarshal(raw, &t) == nil {
			for _, m := range t.Models {
				models = append(models, m.Name)
			}
		}
	} else if raw, err := get("/v1/models"); err == nil { // OpenAI-compatible servers
		var t struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &t) == nil {
			for _, m := range t.Data {
				models = append(models, m.ID)
			}
		}
	} else {
		return nil, fmt.Errorf("no local LLM found at %s. Start one (e.g. `ollama serve`)", base)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("a server is running at %s but has no models. Pull one (e.g. `ollama pull llama3.2`)", base)
	}
	return models, nil
}

// Verify makes a tiny call to prove the configuration really works.
func Verify(ctx context.Context, c Client) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := c.Chat(ctx, "", []Message{{"user", "Reply with the single word: OK"}})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return errors.New("the LLM returned an empty reply")
	}
	return nil
}

// ---------- Holder: the one active LLM for this server process ----------

type Holder struct {
	mu sync.RWMutex
	c  Client
}

func (h *Holder) Set(c Client) { h.mu.Lock(); h.c = c; h.mu.Unlock() }
func (h *Holder) Reset()       { h.Set(nil) }
func (h *Holder) Get() Client  { h.mu.RLock(); defer h.mu.RUnlock(); return h.c }
