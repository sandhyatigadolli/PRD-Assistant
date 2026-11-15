package agent

import (
	"strings"

	"prd-assistant/internal/domain"
	"prd-assistant/internal/templates"
)

// Prompt is what the UI should ask next.
type Prompt struct {
	Kind         string `json:"kind"` // question | repeat | done
	Section      string `json:"section,omitempty"`
	SectionTitle string `json:"section_title,omitempty"`
	SectionIndex int    `json:"section_index"`
	SectionCount int    `json:"section_count"`
	Item         int    `json:"item"`
	ItemLabel    string `json:"item_label,omitempty"`
	Field        string `json:"field,omitempty"`
	Label        string `json:"label,omitempty"`
	Text         string `json:"text,omitempty"`
	Hint         string `json:"hint,omitempty"`
	Required     bool   `json:"required"`
	Priority     bool   `json:"priority"`
	Noun         string `json:"noun,omitempty"`
}

type key struct {
	section string
	item    int
	field   string
}

func index(answers []domain.Answer) map[key]*domain.Answer {
	m := make(map[key]*domain.Answer, len(answers))
	for i := range answers {
		a := &answers[i]
		m[key{a.Section, a.Item, a.Field}] = a
	}
	return m
}

func itemCount(s *templates.Section, sess *domain.Session) int {
	if s.Repeatable == "" {
		return 1
	}
	return max(sess.Repeat[s.Key].Items, 1)
}

func itemLabel(s *templates.Section, item int) string {
	if s.Repeatable == "" {
		return ""
	}
	return capitalize(s.Repeatable) + " " + itoa(item+1)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Next is the planner: it decides what to ask next, in template order.
// A question is pending when it has no answer yet, or its answer still needs review.
func Next(tpl *templates.Template, sess *domain.Session, answers []domain.Answer) Prompt {
	idx := index(answers)
	n := len(tpl.Sections)
	for si := range tpl.Sections {
		s := &tpl.Sections[si]
		for it := 0; it < itemCount(s, sess); it++ {
			for qi := range s.Questions {
				q := &s.Questions[qi]
				a, ok := idx[key{s.Key, it, q.Key}]
				if ok && a.Status != domain.StatusNeedsReview {
					continue
				}
				return Prompt{Kind: "question", Section: s.Key, SectionTitle: s.Title, SectionIndex: si + 1,
					SectionCount: n, Item: it, ItemLabel: itemLabel(s, it), Field: q.Key, Label: q.Label,
					Text: q.Prompt, Hint: q.Hint, Required: q.Required, Priority: q.Priority}
			}
		}
		if s.Repeatable != "" && !sess.Repeat[s.Key].Closed {
			return Prompt{Kind: "repeat", Section: s.Key, SectionTitle: s.Title, SectionIndex: si + 1,
				SectionCount: n, Noun: s.Repeatable, Item: itemCount(s, sess) - 1}
		}
	}
	return Prompt{Kind: "done", SectionIndex: n, SectionCount: n}
}
