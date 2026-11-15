package tools

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Exporter renders the PRD markdown to .docx (hand-built OOXML, no heavy
// dependency) and .pdf (go-pdf/fpdf).
type Exporter struct{}

func (Exporter) Name() string        { return "export" }
func (Exporter) Description() string { return "Exports the PRD as a Word (.docx) or PDF file." }

type block struct {
	Kind string // h1 h2 h3 p bullet number
	Num  string
	Text string
}

var (
	numRe   = regexp.MustCompile(`^(\d+)[.)]\s+(.*)$`)
	boldRe  = regexp.MustCompile(`\*\*(.+?)\*\*`)
	sepRe   = regexp.MustCompile(`^\|?[\s:\-|]+\|?$`)
	codeRe  = regexp.MustCompile("^```")
	inlineR = regexp.MustCompile("`([^`]+)`")
)

// parseMarkdown handles the small subset the app and LLM prompt produce.
func parseMarkdown(md string) []block {
	var out []block
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		t := strings.TrimRight(line, " \t")
		s := strings.TrimSpace(t)
		if s == "" || codeRe.MatchString(s) {
			continue
		}
		s = inlineR.ReplaceAllString(s, "$1")
		switch {
		case strings.HasPrefix(s, "### "):
			out = append(out, block{Kind: "h3", Text: s[4:]})
		case strings.HasPrefix(s, "## "):
			out = append(out, block{Kind: "h2", Text: s[3:]})
		case strings.HasPrefix(s, "# "):
			out = append(out, block{Kind: "h1", Text: s[2:]})
		case strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* "):
			out = append(out, block{Kind: "bullet", Text: strings.TrimSpace(s[2:])})
		case numRe.MatchString(s):
			m := numRe.FindStringSubmatch(s)
			out = append(out, block{Kind: "number", Num: m[1], Text: m[2]})
		case strings.HasPrefix(s, "|"):
			if sepRe.MatchString(s) {
				continue
			}
			out = append(out, block{Kind: "p", Text: strings.TrimSpace(strings.ReplaceAll(strings.Trim(s, "|"), "|", " | "))})
		default:
			out = append(out, block{Kind: "p", Text: s})
		}
	}
	return out
}

type run struct {
	Text string
	Bold bool
}

func runs(text string) []run {
	var out []run
	last := 0
	for _, m := range boldRe.FindAllStringSubmatchIndex(text, -1) {
		if m[0] > last {
			out = append(out, run{Text: text[last:m[0]]})
		}
		out = append(out, run{Text: text[m[2]:m[3]], Bold: true})
		last = m[1]
	}
	if last < len(text) {
		out = append(out, run{Text: text[last:]})
	}
	return out
}

func (e Exporter) Export(format, markdown string) (data []byte, contentType, ext string, err error) {
	blocks := parseMarkdown(markdown)
	switch format {
	case "docx":
		d, err := buildDocx(blocks)
		return d, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	case "pdf":
		d, err := buildPDF(blocks)
		return d, "application/pdf", "pdf", err
	}
	return nil, "", "", fmt.Errorf("unsupported format %q (use docx or pdf)", format)
}

// ---------------- DOCX ----------------

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func docxRuns(text string) string {
	var sb strings.Builder
	for _, r := range runs(text) {
		sb.WriteString("<w:r>")
		if r.Bold {
			sb.WriteString("<w:rPr><w:b/></w:rPr>")
		}
		sb.WriteString(`<w:t xml:space="preserve">` + esc(r.Text) + `</w:t></w:r>`)
	}
	return sb.String()
}

func buildDocx(blocks []block) ([]byte, error) {
	var body strings.Builder
	for _, b := range blocks {
		switch b.Kind {
		case "h1":
			body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr>` + docxRuns(b.Text) + `</w:p>`)
		case "h2":
			body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr>` + docxRuns(b.Text) + `</w:p>`)
		case "h3":
			body.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr>` + docxRuns(b.Text) + `</w:p>`)
		case "bullet":
			body.WriteString(`<w:p><w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>` + docxRuns("•  "+b.Text) + `</w:p>`)
		case "number":
			body.WriteString(`<w:p><w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>` + docxRuns(b.Num+".  "+b.Text) + `</w:p>`)
		default:
			body.WriteString(`<w:p>` + docxRuns(b.Text) + `</w:p>`)
		}
	}
	const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document ` + ns + `><w:body>` + body.String() +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1300" w:bottom="1440" w:left="1300" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`
	heading := func(id, name string, size int, color string) string {
		return fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="%s"><w:name w:val="%s"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="320" w:after="120"/></w:pPr><w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr></w:style>`, id, name, color, size)
	}
	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles ` + ns + `>` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>` +
		heading("Title", "Title", 48, "1F2A44") + heading("Heading1", "heading 1", 32, "2B4C9B") + heading("Heading2", "heading 2", 26, "2B4C9B") +
		`</w:styles>`
	files := []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"word/document.xml", doc},
		{"word/styles.xml", styles},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------- PDF ----------------

func buildPDF(blocks []block) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	tr := pdf.UnicodeTranslatorFromDescriptor("") // core fonts are latin-1/cp1252 only
	pdf.AddPage()

	write := func(h float64, size float64, text string, forceBold bool) {
		for _, r := range runs(text) {
			style := ""
			if r.Bold || forceBold {
				style = "B"
			}
			pdf.SetFont("Helvetica", style, size)
			pdf.Write(h, tr(r.Text))
		}
	}
	for _, b := range blocks {
		switch b.Kind {
		case "h1":
			pdf.SetTextColor(31, 42, 68)
			write(11, 22, b.Text, true)
			pdf.Ln(14)
		case "h2":
			pdf.Ln(3)
			pdf.SetTextColor(43, 76, 155)
			write(8, 15, b.Text, true)
			pdf.Ln(10)
		case "h3":
			pdf.SetTextColor(43, 76, 155)
			write(7, 12, b.Text, true)
			pdf.Ln(8)
		case "bullet", "number":
			pdf.SetTextColor(30, 30, 30)
			pdf.SetLeftMargin(28)
			pdf.SetX(28)
			marker := "•  "
			if b.Kind == "number" {
				marker = b.Num + ".  "
			}
			pdf.SetFont("Helvetica", "", 11)
			pdf.SetX(23)
			pdf.Write(6, tr(marker))
			write(6, 11, b.Text, false)
			pdf.Ln(7)
			pdf.SetLeftMargin(20)
		default:
			pdf.SetTextColor(30, 30, 30)
			write(6, 11, b.Text, false)
			pdf.Ln(8)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
