package llm

import (
	"fmt"
	"strings"
)

// ReviewSystem is the system prompt for checking one answer against PRD quality rules.
const ReviewSystem = `You are a strict but helpful senior product manager reviewing ONE answer in a PRD being written.
You must reply with ONLY a JSON object, no prose, no code fences:
{"ok": boolean, "issues": [string], "suggestion": string, "extracted": {"<field_key>": string}}

Rules:
- "ok" is true when the answer satisfies ALL the quality rules given and is specific enough for a PRD.
- "issues": short, concrete problems (max 3). Empty when ok.
- "suggestion": a rewritten, improved version of the user's answer that fixes the issues WITHOUT inventing facts. Use placeholders like [X%] or [date] where the user must supply a number. Empty when ok.
- "extracted": if the user's answer ALSO clearly answers other questions listed under "Other unanswered questions in this section", put the relevant text for those field keys here. Only include fields that are really answered. Otherwise {}.
- Be lenient on optional questions: ok=true unless the answer is empty, meaningless or contradicts earlier answers.`

func ReviewUser(prdType, section string, question, label string, rules []string, answer, others, context string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PRD type: %s\nSection: %s\nQuestion (%s): %s\n", prdType, section, label, question)
	if len(rules) > 0 {
		b.WriteString("Quality rules:\n")
		for _, r := range rules {
			b.WriteString("- " + r + "\n")
		}
	} else {
		b.WriteString("Quality rules:\n- The answer is specific and meaningful.\n")
	}
	if others != "" {
		b.WriteString("Other unanswered questions in this section (field_key: question):\n" + others + "\n")
	}
	if context != "" {
		b.WriteString("Earlier answers for context:\n" + context + "\n")
	}
	b.WriteString("\nUser's answer:\n" + answer)
	return b.String()
}

const RewriteSystem = `You edit one block of a PRD according to the user's instruction.
Reply with ONLY the new text of the block (no quotes, no preamble, no explanation).
Keep the user's facts, do not invent numbers or names; use placeholders like [X%] if needed.`

func RewriteUser(question, current, instruction, context string) string {
	return fmt.Sprintf("Question this block answers: %s\n\nCurrent text:\n%s\n\nInstruction: %s\n\nEarlier answers for context:\n%s",
		question, current, instruction, context)
}

const GenerateSystem = `You are a senior product manager. Turn the structured answers below into a polished, professional PRD in Markdown.
Rules:
- Use "# <Title>" then "## <Section>" headings in EXACTLY the section order given.
- Preserve every fact, number and name. Do NOT invent facts; if something is missing write "TBD".
- Keep priority/release tags (e.g. [P0], [V1]) next to the item they belong to.
- Use short paragraphs and "- " bullet lists. Number lists with "1. ". No tables, no code fences, no HTML.
- Start with a 2-3 sentence "## Executive Summary" synthesised from the answers.
- Output ONLY the Markdown document.`
