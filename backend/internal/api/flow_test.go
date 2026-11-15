package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"prd-assistant/internal/api"
	"prd-assistant/internal/config"
	"prd-assistant/internal/store"
	"prd-assistant/internal/templates"
)

// mockLLM is a tiny OpenAI-compatible server with scripted behaviour.
func mockLLM() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"mock-model"}]}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		system, user := "", ""
		for _, m := range req.Messages {
			if m.Role == "system" {
				system = m.Content
			} else {
				user = m.Content
			}
		}
		reply := "OK"
		switch {
		case strings.Contains(system, "reviewing ONE answer"):
			ans := user[strings.LastIndex(user, "User's answer:"):]
			switch {
			case strings.Contains(ans, "EXTRACT"):
				reply = `{"ok":true,"issues":[],"suggestion":"","extracted":{"author":"Priya","status":"Draft"}}`
			case strings.Contains(ans, "weak"):
				reply = "```json\n{\"ok\":false,\"issues\":[\"Not measurable\"],\"suggestion\":\"Reduce X from [A] to [B]\",\"extracted\":{}}\n```"
			default:
				reply = `{"ok":true,"issues":[],"suggestion":"","extracted":{}}`
			}
		case strings.Contains(system, "edit one block"):
			reply = "Rewritten by AI"
		case strings.Contains(system, "polished, professional PRD"):
			reply = "# Polished\n\n## Executive Summary\n\nHello **world**.\n\n- one\n- two\n"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	})
	return httptest.NewServer(mux)
}

type client struct {
	t *testing.T
	h http.Handler
}

func (c client) do(method, path string, body any) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func (c client) json(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	code, b := c.do(method, path, body)
	if code != want {
		c.t.Fatalf("%s %s: got %d want %d: %s", method, path, code, want, b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func prompt(st map[string]any) map[string]any { return st["prompt"].(map[string]any) }

func TestFullFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := mockLLM()
	defer mock.Close()
	tpls, err := templates.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := client{t, api.New(config.Config{OllamaURL: mock.URL}, store.NewMemory(), tpls)}

	// 1. gate: nothing works without an LLM
	if s := c.json("GET", "/api/llm/status", nil, 200); s["ready"] != false {
		t.Fatal("should not be ready")
	}
	c.json("POST", "/api/sessions", map[string]string{"prd_type": "product"}, 400)

	// bad API key / unreachable local -> clear error + hint, stays unready
	bad := c.json("POST", "/api/llm/configure", map[string]any{"mode": "local", "base_url": "http://127.0.0.1:1"}, 400)
	if bad["hint"] == nil {
		t.Fatal("expected hint")
	}
	c.json("POST", "/api/llm/configure", map[string]any{"mode": "api", "provider": "openai", "api_key": ""}, 400)

	// local LLM check works
	loc := c.json("GET", "/api/llm/local?base_url="+mock.URL, nil, 200)
	if loc["reachable"] != true {
		t.Fatalf("local not reachable: %v", loc)
	}
	st := c.json("POST", "/api/llm/configure", map[string]any{"mode": "local", "base_url": mock.URL}, 200)
	if st["ready"] != true {
		t.Fatal("should be ready")
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), "api_key") {
		t.Fatal("status must never expose key")
	}

	// 2. session + first question
	s := c.json("POST", "/api/sessions", map[string]string{"prd_type": "product"}, 201)
	sid := s["session"].(map[string]any)["id"].(string)
	if prompt(s)["field"] != "product_name" {
		t.Fatalf("first question: %v", prompt(s))
	}
	// required questions cannot be skipped
	c.json("POST", "/api/sessions/"+sid+"/skip", nil, 400)

	// 3. answer many at once -> author/status are filled, next is reviewers
	s = c.json("POST", "/api/sessions/"+sid+"/answer", map[string]any{"content": "Acme Clinic EXTRACT", "tags": []string{"IMPORTANT"}}, 200)
	if prompt(s)["field"] != "reviewers" {
		t.Fatalf("extraction should skip to reviewers, got %v", prompt(s)["field"])
	}
	s = c.json("POST", "/api/sessions/"+sid+"/skip", nil, 200)
	if prompt(s)["field"] != "problem_statement" {
		t.Fatalf("expected problem_statement, got %v", prompt(s)["field"])
	}
	// 4. weak answer -> needs_review, same question again; then keep as is
	s = c.json("POST", "/api/sessions/"+sid+"/answer", map[string]any{"content": "weak problem"}, 200)
	if prompt(s)["field"] != "problem_statement" {
		t.Fatal("weak answer must re-ask")
	}
	pv := c.json("GET", "/api/sessions/"+sid+"/preview", nil, 200)
	aid := findAnswer(pv, "problem_statement", 0)
	s = c.json("POST", "/api/sessions/"+sid+"/answers/"+aid+"/accept", nil, 200)
	if prompt(s)["field"] == "problem_statement" {
		t.Fatal("accept must advance")
	}

	// 5. walk the whole template
	reqSeen, repeats := 0, 0
	for i := 0; i < 200; i++ {
		p := prompt(s)
		switch p["kind"] {
		case "done":
			goto done
		case "repeat":
			add := p["section"] == "requirements" && repeats == 0
			if add {
				repeats++
			}
			s = c.json("POST", "/api/sessions/"+sid+"/repeat", map[string]bool{"add": add}, 200)
		case "question":
			if p["required"] == true {
				tags := []string{}
				if p["priority"] == true {
					// missing priority -> flagged, then retry with tag
					s = c.json("POST", "/api/sessions/"+sid+"/answer", map[string]any{"content": "fine"}, 200)
					if prompt(s)["field"] != p["field"] {
						t.Fatal("priority rule should re-ask")
					}
					tags = []string{"P0", "V1"}
					reqSeen++
				}
				s = c.json("POST", "/api/sessions/"+sid+"/answer", map[string]any{"content": "fine answer " + p["field"].(string), "tags": tags}, 200)
			} else {
				s = c.json("POST", "/api/sessions/"+sid+"/skip", map[string]string{"scope": "section"}, 200)
			}
		}
	}
	t.Fatal("never reached done")
done:
	if reqSeen != 2 {
		t.Fatalf("expected 2 requirement items, got %d", reqSeen)
	}

	// 6. edit: propose (diff) -> apply, history keeps last 3
	pv = c.json("GET", "/api/sessions/"+sid+"/preview", nil, 200)
	aid = findAnswer(pv, "problem_statement", 0)
	pr := c.json("POST", "/api/sessions/"+sid+"/answers/"+aid+"/propose", map[string]any{"content": "Clinics lose 25% to no-shows"}, 200)
	if pr["changed"] != true || len(pr["diff"].([]any)) < 2 {
		t.Fatalf("bad proposal: %v", pr)
	}
	ai := c.json("POST", "/api/sessions/"+sid+"/answers/"+aid+"/propose", map[string]any{"instruction": "make measurable"}, 200)
	if ai["new"] != "Rewritten by AI" {
		t.Fatalf("AI rewrite: %v", ai["new"])
	}
	for i := 0; i < 5; i++ {
		c.json("POST", "/api/sessions/"+sid+"/answers/"+aid+"/apply", map[string]any{"content": "Edit v" + string(rune('A'+i))}, 200)
	}
	code, b := c.do("GET", "/api/sessions/"+sid+"/answers/"+aid+"/history", nil)
	var hist []map[string]any
	_ = json.Unmarshal(b, &hist)
	if code != 200 || len(hist) != 3 || hist[0]["content"] != "Edit vD" {
		t.Fatalf("history should hold the last 3 (newest first): %s", b)
	}
	// required block can't be emptied
	c.json("POST", "/api/sessions/"+sid+"/answers/"+aid+"/apply", map[string]any{"content": " "}, 400)

	// 7. preview + tag filter
	pv = c.json("GET", "/api/sessions/"+sid+"/preview", nil, 200)
	md := pv["markdown"].(string)
	if !strings.Contains(md, "Acme Clinic") || !strings.Contains(md, "[P0] [V1]") || !strings.Contains(md, "### Requirement 2") {
		t.Fatalf("markdown missing content:\n%s", md)
	}
	pv = c.json("GET", "/api/sessions/"+sid+"/preview?tags=V2", nil, 200)
	if strings.Contains(pv["markdown"].(string), "[P0] [V1]") || !strings.Contains(pv["markdown"].(string), "Edit vE") {
		t.Fatal("tag filter should hide V1-tagged blocks but keep untagged core content")
	}

	// 8. generate + export
	gen := c.json("POST", "/api/sessions/"+sid+"/generate", nil, 200)
	if !strings.Contains(gen["markdown"].(string), "Executive Summary") {
		t.Fatal("generate")
	}
	for _, src := range []string{"answers", "polished"} {
		code, docx := c.do("GET", "/api/sessions/"+sid+"/export?format=docx&source="+src, nil)
		zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
		if code != 200 || err != nil {
			t.Fatalf("docx %s: %d %v", src, code, err)
		}
		found := false
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				found = true
				rc, _ := f.Open()
				x, _ := io.ReadAll(rc)
				if !strings.Contains(string(x), "<w:document") {
					t.Fatal("bad document.xml")
				}
			}
		}
		if !found {
			t.Fatal("no document.xml")
		}
		code, pdf := c.do("GET", "/api/sessions/"+sid+"/export?format=pdf&source="+src, nil)
		if code != 200 || !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatalf("pdf %s: %d", src, code)
		}
	}
	c.json("GET", "/api/sessions/"+sid+"/export?format=rtf", nil, 400)

	// 9. context survives: reloading the session returns the whole chat
	s = c.json("GET", "/api/sessions/"+sid, nil, 200)
	if len(s["messages"].([]any)) < 20 || prompt(s)["kind"] != "done" {
		t.Fatal("state should be fully recoverable")
	}
}

func findAnswer(pv map[string]any, field string, item int) string {
	for _, sec := range pv["sections"].([]any) {
		for _, it := range sec.(map[string]any)["items"].([]any) {
			m := it.(map[string]any)
			if int(m["index"].(float64)) != item {
				continue
			}
			for _, b := range m["blocks"].([]any) {
				bm := b.(map[string]any)
				if bm["field"] == field && bm["answer_id"] != nil {
					return bm["answer_id"].(string)
				}
			}
		}
	}
	return ""
}
