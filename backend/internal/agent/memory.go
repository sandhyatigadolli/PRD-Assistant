package agent

import (
	"fmt"
	"strings"

	"prd-assistant/internal/domain"
	"prd-assistant/internal/templates"
)

// Context rebuilds the agent's working memory from the stored answers. Because
// the database is the source of truth, nothing is lost across edits, page
// reloads or server restarts: every LLM call gets the up-to-date PRD so far.
func Context(tpl *templates.Template, answers []domain.Answer, excludeID string, perAnswer int) string {
	idx := index(answers)
	var b strings.Builder
	for si := range tpl.Sections {
		s := &tpl.Sections[si]
		for it := 0; it < 50; it++ {
			found := false
			for qi := range s.Questions {
				q := &s.Questions[qi]
				a, ok := idx[key{s.Key, it, q.Key}]
				if !ok {
					continue
				}
				found = true
				if a.ID == excludeID || a.Skipped || strings.TrimSpace(a.Content) == "" {
					continue
				}
				c := strings.TrimSpace(a.Content)
				if len(c) > perAnswer {
					c = c[:perAnswer] + "..."
				}
				lbl := q.Label
				if l := itemLabel(s, it); l != "" {
					lbl = l + " - " + lbl
				}
				fmt.Fprintf(&b, "- %s: %s\n", lbl, strings.ReplaceAll(c, "\n", " "))
			}
			if !found {
				break
			}
		}
	}
	return b.String()
}
