// Package agent is the PRD-building agent: a loop of
// plan (which question next) -> ask -> review answer (LLM tool) -> store -> preview.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"prd-assistant/internal/domain"
	"prd-assistant/internal/llm"
	"prd-assistant/internal/store"
	"prd-assistant/internal/templates"
	"prd-assistant/internal/tools"
)

// UserError is a problem the caller can fix (maps to HTTP 400/409).
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

func bad(f string, a ...any) error { return &UserError{fmt.Sprintf(f, a...)} }

type Agent struct {
	Store     store.Store
	Templates *templates.Registry
	Tools     *tools.Registry
	LLM       *llm.Holder
}

type State struct {
	Session  *domain.Session  `json:"session"`
	Messages []domain.Message `json:"messages"`
	Prompt   Prompt           `json:"prompt"`
}

func (a *Agent) load(ctx context.Context, sid string) (*domain.Session, *templates.Template, []domain.Answer, error) {
	s, err := a.Store.GetSession(ctx, sid)
	if err != nil {
		return nil, nil, nil, err
	}
	tpl, ok := a.Templates.Get(s.PRDType)
	if !ok {
		return nil, nil, nil, bad("unknown PRD type %q", s.PRDType)
	}
	ans, err := a.Store.ListAnswers(ctx, sid)
	return s, tpl, ans, err
}

func (a *Agent) say(ctx context.Context, sid, role, text string) {
	_ = a.Store.AddMessage(ctx, &domain.Message{SessionID: sid, Role: role, Content: text})
}

func (a *Agent) requireLLM() error {
	if a.LLM.Get() == nil {
		return bad("no LLM is configured. Set an API key or connect a local LLM first")
	}
	return nil
}

// ---------- session lifecycle ----------

func (a *Agent) StartSession(ctx context.Context, prdType string) (*State, error) {
	if err := a.requireLLM(); err != nil {
		return nil, err
	}
	tpl, ok := a.Templates.Get(prdType)
	if !ok {
		return nil, bad("prd_type must be one of: product, project, feature")
	}
	s := &domain.Session{Title: "", PRDType: prdType, Repeat: map[string]domain.RepeatState{}}
	if err := a.Store.CreateSession(ctx, s); err != nil {
		return nil, err
	}
	a.say(ctx, s.ID, "agent", fmt.Sprintf("Great, let's build a **%s**. I'll ask one question at a time, in PRD order. "+
		"Answer by typing or with the mic, add tags if you like, and watch the preview fill in on the right. "+
		"You can click any block in the preview to edit it.", tpl.Title))
	return a.advance(ctx, s.ID)
}

func (a *Agent) GetState(ctx context.Context, sid string) (*State, error) {
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	msgs, err := a.Store.ListMessages(ctx, sid)
	if err != nil {
		return nil, err
	}
	return &State{Session: s, Messages: msgs, Prompt: Next(tpl, s, ans)}, nil
}

// advance asks the next question (writes it to the chat) and returns the state.
func (a *Agent) advance(ctx context.Context, sid string) (*State, error) {
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	p := Next(tpl, s, ans)
	sec, _ := tpl.Section(p.Section)
	var b strings.Builder
	switch p.Kind {
	case "done":
		b.WriteString("That's every question! 🎉 Review the preview, click any block to refine it, " +
			"then **Generate polished PRD** and download it as Word or PDF.")
	case "repeat":
		fmt.Fprintf(&b, "Would you like to add another **%s**?", p.Noun)
	case "question":
		idx := index(ans)
		firstOfSection := p.Field == sec.Questions[0].Key
		_, startedSection := idx[key{p.Section, p.Item, p.Field}]
		if firstOfSection && !startedSection {
			if p.Item == 0 {
				fmt.Fprintf(&b, "### %s\n", p.SectionTitle)
				if sec.Intro != "" {
					b.WriteString(sec.Intro + "\n\n")
				}
			}
			if p.ItemLabel != "" {
				fmt.Fprintf(&b, "**%s**\n\n", p.ItemLabel)
			}
		}
		b.WriteString(p.Text)
		if p.Hint != "" {
			b.WriteString("\n\n_" + p.Hint + "_")
		}
		if !p.Required {
			b.WriteString("\n\n_(optional, you can skip this)_")
		}
	}
	a.say(ctx, sid, "agent", b.String())
	return a.GetState(ctx, sid)
}

// ---------- answering ----------

func (a *Agent) Submit(ctx context.Context, sid, content string, tags []string) (*State, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, bad("answer is empty")
	}
	tags, err := cleanTags(tags)
	if err != nil {
		return nil, err
	}
	if err := a.requireLLM(); err != nil {
		return nil, err
	}
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	p := Next(tpl, s, ans)
	if p.Kind != "question" {
		return nil, bad("there is no open question (current step: %s)", p.Kind)
	}
	sec, _ := tpl.Section(p.Section)
	q, _ := sec.Question(p.Field)
	idx := index(ans)

	// Upsert the answer for this question. Re-answers keep the old value as a version.
	cur := idx[key{p.Section, p.Item, p.Field}]
	if cur == nil {
		cur = &domain.Answer{SessionID: sid, Section: p.Section, Item: p.Item, Field: p.Field}
	} else if cur.Content != content {
		_ = a.Store.AddVersion(ctx, &domain.AnswerVersion{AnswerID: cur.ID, Content: cur.Content, Tags: cur.Tags})
	}
	cur.Content, cur.Tags, cur.Skipped = content, tags, false
	a.say(ctx, sid, "user", content+tagLine(tags))

	// Review with the validator tool.
	var others []templates.Question
	for _, oq := range sec.Questions {
		if oq.Key == q.Key {
			continue
		}
		if _, ok := idx[key{p.Section, p.Item, oq.Key}]; !ok {
			others = append(others, oq)
		}
	}
	rev, rerr := a.Tools.Validator.Review(ctx, tools.ReviewInput{
		PRDType: tpl.Title, Section: sec, Question: q, Answer: content, Tags: tags,
		Others: others, Context: Context(tpl, ans, cur.ID, 300),
	})
	warn := ""
	if rerr != nil {
		warn = "⚠️ I couldn't run the quality check (" + rerr.Error() + "), so I saved your answer as is."
	}
	cur.Feedback, cur.Suggest = nil, ""
	if rev != nil && !rev.OK && rerr == nil {
		cur.Status, cur.Feedback, cur.Suggest = domain.StatusNeedsReview, rev.Issues, rev.Suggestion
	} else {
		cur.Status = domain.StatusAccepted
		if rev != nil && !rev.OK { // deterministic issue (e.g. missing priority) but LLM unavailable
			cur.Status, cur.Feedback = domain.StatusNeedsReview, rev.Issues
		}
	}
	if err := a.Store.SaveAnswer(ctx, cur); err != nil {
		return nil, err
	}

	// Title the session from the very first answer (product/project/feature name).
	if s.Title == "" && p.SectionIndex == 1 && p.Field == sec.Questions[0].Key {
		s.Title = truncate(firstLine(content), 70)
		_ = a.Store.UpdateSession(ctx, s)
	}

	// Answers volunteered for other questions in the same section.
	var filled []string
	if rev != nil && cur.Status == domain.StatusAccepted {
		for _, oq := range others {
			if txt, ok := rev.Extracted[oq.Key]; ok {
				_ = a.Store.SaveAnswer(ctx, &domain.Answer{SessionID: sid, Section: p.Section, Item: p.Item,
					Field: oq.Key, Content: txt, Tags: tags, Status: domain.StatusAccepted})
				filled = append(filled, oq.Label)
			}
		}
	}

	if cur.Status == domain.StatusNeedsReview {
		a.say(ctx, sid, "agent", reviewMessage(cur))
		return a.GetState(ctx, sid)
	}
	var note strings.Builder
	if warn != "" {
		note.WriteString(warn + "\n\n")
	}
	note.WriteString("✅ Saved.")
	if len(filled) > 0 {
		note.WriteString(" I also filled in: " + strings.Join(filled, ", ") + ", so I'll skip those questions.")
	}
	a.say(ctx, sid, "agent", note.String())
	return a.advance(ctx, sid)
}

func reviewMessage(a *domain.Answer) string {
	var b strings.Builder
	b.WriteString("I saved your answer, but it could be stronger:\n")
	for _, i := range a.Feedback {
		b.WriteString("- " + i + "\n")
	}
	if a.Suggest != "" {
		b.WriteString("\n**Suggested rewrite:**\n> " + strings.ReplaceAll(a.Suggest, "\n", "\n> ") + "\n")
	}
	b.WriteString("\nRevise your answer below, or choose **Keep as is** to move on.")
	return b.String()
}

// Accept keeps an answer flagged by the validator as is.
func (a *Agent) Accept(ctx context.Context, sid, aid string) (*State, error) {
	ans, err := a.Store.GetAnswer(ctx, sid, aid)
	if err != nil {
		return nil, err
	}
	ans.Status, ans.Feedback, ans.Suggest = domain.StatusAccepted, nil, ""
	if err := a.Store.SaveAnswer(ctx, ans); err != nil {
		return nil, err
	}
	a.say(ctx, sid, "user", "Keep as is.")
	return a.advance(ctx, sid)
}

func (a *Agent) Skip(ctx context.Context, sid string, wholeSection bool) (*State, error) {
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	p := Next(tpl, s, ans)
	if p.Kind != "question" {
		return nil, bad("there is no open question to skip")
	}
	if p.Required {
		return nil, bad("this question is required for a complete PRD and can't be skipped")
	}
	sec, _ := tpl.Section(p.Section)
	idx := index(ans)
	skipOne := func(q templates.Question) error {
		return a.Store.SaveAnswer(ctx, &domain.Answer{SessionID: sid, Section: p.Section, Item: p.Item,
			Field: q.Key, Skipped: true, Status: domain.StatusAccepted, Tags: []string{}})
	}
	if !wholeSection {
		if err := skipOne(*mustQ(sec, p.Field)); err != nil {
			return nil, err
		}
		a.say(ctx, sid, "user", "Skip.")
		return a.advance(ctx, sid)
	}
	for _, q := range sec.Questions {
		if _, ok := idx[key{p.Section, p.Item, q.Key}]; ok || q.Required {
			continue
		}
		if err := skipOne(q); err != nil {
			return nil, err
		}
	}
	a.say(ctx, sid, "user", "Skip the rest of this section.")
	return a.advance(ctx, sid)
}

func mustQ(s *templates.Section, k string) *templates.Question { q, _ := s.Question(k); return q }

func (a *Agent) Repeat(ctx context.Context, sid string, add bool) (*State, error) {
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	p := Next(tpl, s, ans)
	if p.Kind != "repeat" {
		return nil, bad("nothing to repeat right now")
	}
	sec, _ := tpl.Section(p.Section)
	st := s.Repeat[p.Section]
	st.Items = itemCount(sec, s)
	if add {
		st.Items++
		a.say(ctx, sid, "user", "Yes, add another "+p.Noun+".")
	} else {
		st.Closed = true
		a.say(ctx, sid, "user", "No, that's all.")
	}
	s.Repeat[p.Section] = st
	if err := a.Store.UpdateSession(ctx, s); err != nil {
		return nil, err
	}
	return a.advance(ctx, sid)
}

// ---------- editing (GitHub-style propose -> confirm) ----------

type Proposal struct {
	AnswerID string              `json:"answer_id"`
	Label    string              `json:"label"`
	Old      string              `json:"old"`
	New      string              `json:"new"`
	OldTags  []string            `json:"old_tags"`
	NewTags  []string            `json:"new_tags"`
	Diff     []tools.DiffSegment `json:"diff"`
	Changed  bool                `json:"changed"`
}

type ProposeInput struct {
	Content     *string  `json:"content"`
	Tags        []string `json:"tags"`
	Instruction string   `json:"instruction"` // ask the AI to rewrite
}

func (a *Agent) Propose(ctx context.Context, sid, aid string, in ProposeInput) (*Proposal, error) {
	s, tpl, all, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	cur, err := a.Store.GetAnswer(ctx, sid, aid)
	if err != nil {
		return nil, err
	}
	q := mustQ(mustS(tpl, cur.Section), cur.Field)
	_ = s
	newText := cur.Content
	switch {
	case in.Content != nil:
		newText = strings.TrimSpace(*in.Content)
	case strings.TrimSpace(in.Instruction) != "":
		if err := a.requireLLM(); err != nil {
			return nil, err
		}
		out, err := a.LLM.Get().Chat(ctx, llm.RewriteSystem, []llm.Message{{Role: "user",
			Content: llm.RewriteUser(q.Prompt, cur.Content, in.Instruction, Context(tpl, all, cur.ID, 300))}})
		if err != nil {
			return nil, err
		}
		newText = strings.TrimSpace(strings.Trim(strings.TrimSpace(out), "`"))
	}
	newTags := cur.Tags
	if in.Tags != nil {
		if newTags, err = cleanTags(in.Tags); err != nil {
			return nil, err
		}
	}
	p := &Proposal{AnswerID: aid, Label: q.Label, Old: cur.Content, New: newText, OldTags: nz(cur.Tags), NewTags: nz(newTags)}
	p.Diff = a.Tools.Differ.Diff(ctx, cur.Content, newText)
	p.Changed = cur.Content != newText || strings.Join(p.OldTags, ",") != strings.Join(p.NewTags, ",")
	return p, nil
}

func mustS(t *templates.Template, k string) *templates.Section { s, _ := t.Section(k); return s }

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Apply commits an edit. The previous value is snapshotted (last 3 are kept).
func (a *Agent) Apply(ctx context.Context, sid, aid, content string, tags []string) (*domain.Answer, error) {
	_, tpl, _, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	cur, err := a.Store.GetAnswer(ctx, sid, aid)
	if err != nil {
		return nil, err
	}
	q := mustQ(mustS(tpl, cur.Section), cur.Field)
	content = strings.TrimSpace(content)
	if content == "" && q.Required {
		return nil, bad("%q is required and can't be empty", q.Label)
	}
	tags, err = cleanTags(tags)
	if err != nil {
		return nil, err
	}
	if cur.Content == content && strings.Join(cur.Tags, ",") == strings.Join(tags, ",") {
		return cur, nil // nothing changed
	}
	if err := a.Store.AddVersion(ctx, &domain.AnswerVersion{AnswerID: cur.ID, Content: cur.Content, Tags: cur.Tags}); err != nil {
		return nil, err
	}
	cur.Content, cur.Tags, cur.Skipped = content, tags, content == ""
	cur.Status, cur.Feedback, cur.Suggest = domain.StatusAccepted, nil, ""
	if err := a.Store.SaveAnswer(ctx, cur); err != nil {
		return nil, err
	}
	a.say(ctx, sid, "system", "✏️ Updated **"+q.Label+"**.")
	return cur, nil
}

func (a *Agent) History(ctx context.Context, sid, aid string) ([]domain.AnswerVersion, error) {
	if _, err := a.Store.GetAnswer(ctx, sid, aid); err != nil {
		return nil, err
	}
	return a.Store.ListVersions(ctx, aid)
}

// ---------- preview & generation ----------

func (a *Agent) Preview(ctx context.Context, sid string, filter map[string]bool) (*Preview, error) {
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return nil, err
	}
	p := BuildPreview(tpl, s, ans, filter)
	return &p, nil
}

// Generate asks the LLM to turn the collected answers into a polished PRD.
func (a *Agent) Generate(ctx context.Context, sid string, filter map[string]bool) (string, error) {
	if err := a.requireLLM(); err != nil {
		return "", err
	}
	s, tpl, ans, err := a.load(ctx, sid)
	if err != nil {
		return "", err
	}
	if p := Next(tpl, s, ans); p.Kind != "done" {
		// Allow generating early only if all required questions are answered.
		idx := index(ans)
		for si := range tpl.Sections {
			sec := &tpl.Sections[si]
			for it := 0; it < itemCount(sec, s); it++ {
				for _, q := range sec.Questions {
					if x, ok := idx[key{sec.Key, it, q.Key}]; q.Required && (!ok || x.Skipped) {
						return "", bad("answer the required question %q first (section %q)", q.Label, sec.Title)
					}
				}
			}
		}
	}
	raw := BuildPreview(tpl, s, ans, filter)
	out, err := a.LLM.Get().Chat(ctx, llm.GenerateSystem, []llm.Message{{Role: "user",
		Content: "PRD type: " + tpl.Title + "\n\nStructured answers (section order is final):\n\n" + raw.Markdown}})
	if err != nil {
		return "", err
	}
	md := stripFences(out)
	now := nowUTC()
	s.Generated, s.GeneratedAt = md, &now
	if err := a.Store.UpdateSession(ctx, s); err != nil {
		return "", err
	}
	a.say(ctx, sid, "system", "📄 Generated the polished PRD.")
	return md, nil
}

// ---------- helpers ----------

func cleanTags(tags []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.ToUpper(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		if !domain.ValidTag(t) {
			return nil, bad("unknown tag %q", t)
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

func tagLine(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "\n\n" + strings.TrimSpace(tagText(tags))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = s[strings.Index(s, "\n")+1:]
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

var ErrNotFound = store.ErrNotFound

func IsNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
