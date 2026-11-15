package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"prd-assistant/internal/domain"
	"prd-assistant/internal/llm"
	"prd-assistant/internal/templates"
)

// Review is the validator's verdict on one answer.
type Review struct {
	OK         bool              `json:"ok"`
	Issues     []string          `json:"issues"`
	Suggestion string            `json:"suggestion"`
	Extracted  map[string]string `json:"extracted"`
}

// Validator checks an answer against PRD quality rules (measurable goals, P0-P2
// priorities, ...). Priority is checked deterministically; the rest by the LLM.
type Validator struct{ Holder *llm.Holder }

func (Validator) Name() string { return "review_answer" }
func (Validator) Description() string {
	return "Checks an answer against PRD quality rules via the LLM and extracts answers to other questions."
}

type ReviewInput struct {
	PRDType  string
	Section  *templates.Section
	Question *templates.Question
	Answer   string
	Tags     []string
	Others   []templates.Question // unanswered siblings in the section (for multi-answer extraction)
	Context  string               // summary of earlier answers
}

func (v Validator) Review(ctx context.Context, in ReviewInput) (*Review, error) {
	r := &Review{OK: true, Issues: []string{}, Extracted: map[string]string{}}

	// Deterministic rule: requirement-type answers must carry a P0-P2 priority.
	if in.Question.Priority {
		has := false
		for _, t := range in.Tags {
			if domain.IsPriorityTag(t) {
				has = true
			}
		}
		if !has {
			r.OK = false
			r.Issues = append(r.Issues, "Pick a priority tag (P0, P1 or P2) for this requirement.")
		}
	}

	c := v.Holder.Get()
	if c == nil {
		return r, fmt.Errorf("no LLM configured")
	}
	var others strings.Builder
	for _, q := range in.Others {
		fmt.Fprintf(&others, "%s: %s\n", q.Key, q.Prompt)
	}
	prompt := llm.ReviewUser(in.PRDType, in.Section.Title, in.Question.Prompt, in.Question.Label,
		in.Question.Rules, in.Answer, others.String(), in.Context)
	out, err := c.Chat(ctx, llm.ReviewSystem, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return r, err
	}
	var lr Review
	if err := json.Unmarshal([]byte(extractJSON(out)), &lr); err != nil {
		return r, fmt.Errorf("could not parse the LLM review: %w", err)
	}
	if !lr.OK {
		r.OK = false
	}
	r.Issues = append(r.Issues, lr.Issues...)
	if !lr.OK {
		r.Suggestion = lr.Suggestion
	}
	for k, val := range lr.Extracted {
		if strings.TrimSpace(val) != "" {
			r.Extracted[k] = strings.TrimSpace(val)
		}
	}
	return r, nil
}

// extractJSON pulls the first {...} object out of an LLM reply (handles code fences).
func extractJSON(s string) string {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}
