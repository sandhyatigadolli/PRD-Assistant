package agent

import (
	"fmt"
	"strings"

	"prd-assistant/internal/domain"
	"prd-assistant/internal/templates"
)

type Block struct {
	AnswerID string   `json:"answer_id,omitempty"`
	Field    string   `json:"field"`
	Label    string   `json:"label"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`
	Skipped  bool     `json:"skipped"`
	Pending  bool     `json:"pending"`  // not asked yet
	Included bool     `json:"included"` // passes the current tag filter
	Status   string   `json:"status"`
	Feedback []string `json:"feedback,omitempty"`
	Suggest  string   `json:"suggestion,omitempty"`
	Required bool     `json:"required"`
}

type PreviewItem struct {
	Index  int     `json:"index"`
	Label  string  `json:"label,omitempty"`
	Blocks []Block `json:"blocks"`
}

type PreviewSection struct {
	Key   string        `json:"key"`
	Title string        `json:"title"`
	Items []PreviewItem `json:"items"`
}

type Preview struct {
	Title    string           `json:"title"`
	Sections []PreviewSection `json:"sections"`
	Markdown string           `json:"markdown"`
}

func included(tags []string, filter map[string]bool) bool {
	if len(filter) == 0 || len(tags) == 0 {
		return true // untagged = core content, always included
	}
	for _, t := range tags {
		if filter[t] {
			return true
		}
	}
	return false
}

// BuildPreview renders the live PRD (structured + markdown). Tag filter: if
// non-empty, tagged blocks are included only when they carry one of the tags.
func BuildPreview(tpl *templates.Template, sess *domain.Session, answers []domain.Answer, filter map[string]bool) Preview {
	idx := index(answers)
	title := sess.Title
	if title == "" {
		title = tpl.Title
	}
	p := Preview{Title: title}
	var md strings.Builder
	fmt.Fprintf(&md, "# %s\n\n", title)

	for si := range tpl.Sections {
		s := &tpl.Sections[si]
		ps := PreviewSection{Key: s.Key, Title: s.Title}
		var sec strings.Builder
		for it := 0; it < itemCount(s, sess); it++ {
			pi := PreviewItem{Index: it, Label: itemLabel(s, it)}
			var itemMD strings.Builder
			for qi := range s.Questions {
				q := &s.Questions[qi]
				b := Block{Field: q.Key, Label: q.Label, Tags: []string{}, Included: true, Required: q.Required}
				a, ok := idx[key{s.Key, it, q.Key}]
				if !ok {
					b.Pending = true
					pi.Blocks = append(pi.Blocks, b)
					continue
				}
				b.AnswerID, b.Content, b.Skipped, b.Status, b.Feedback, b.Suggest = a.ID, a.Content, a.Skipped, a.Status, a.Feedback, a.Suggest
				if a.Tags != nil {
					b.Tags = a.Tags
				}
				b.Included = included(a.Tags, filter)
				pi.Blocks = append(pi.Blocks, b)
				if b.Skipped || strings.TrimSpace(b.Content) == "" || !b.Included {
					continue
				}
				fmt.Fprintf(&itemMD, "**%s**%s\n\n%s\n\n", q.Label, tagText(a.Tags), strings.TrimSpace(a.Content))
			}
			ps.Items = append(ps.Items, pi)
			if itemMD.Len() > 0 {
				if pi.Label != "" {
					fmt.Fprintf(&sec, "### %s\n\n", pi.Label)
				}
				sec.WriteString(itemMD.String())
			}
		}
		p.Sections = append(p.Sections, ps)
		if sec.Len() > 0 {
			fmt.Fprintf(&md, "## %s\n\n%s", s.Title, sec.String())
		}
	}
	p.Markdown = strings.TrimSpace(md.String()) + "\n"
	return p
}

func tagText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range tags {
		b.WriteString(" [" + t + "]")
	}
	return b.String()
}
