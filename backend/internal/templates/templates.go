// Package templates loads the PRD question templates (one per PRD type).
// The agent loop is generic; all question content and ordering lives in YAML.
package templates

import (
	"embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed defs/*.yaml
var defs embed.FS

type Question struct {
	Key      string   `yaml:"key" json:"key"`
	Prompt   string   `yaml:"prompt" json:"prompt"`
	Hint     string   `yaml:"hint" json:"hint,omitempty"`
	Required bool     `yaml:"required" json:"required"`
	Priority bool     `yaml:"priority" json:"priority,omitempty"` // must carry a P0-P2 tag
	Rules    []string `yaml:"rules" json:"rules,omitempty"`       // quality rules checked by the LLM
	Label    string   `yaml:"label" json:"label"`                 // short label used in the document
}

type Section struct {
	Key        string     `yaml:"key" json:"key"`
	Title      string     `yaml:"title" json:"title"`
	Intro      string     `yaml:"intro" json:"intro,omitempty"`
	Repeatable string     `yaml:"repeatable" json:"repeatable,omitempty"` // singular noun, e.g. "persona"
	Questions  []Question `yaml:"questions" json:"questions"`
}

type Template struct {
	Type        string    `yaml:"type" json:"type"`
	Title       string    `yaml:"title" json:"title"`
	Description string    `yaml:"description" json:"description"`
	Sections    []Section `yaml:"sections" json:"sections"`
}

type Registry struct{ byType map[string]*Template }

func Load() (*Registry, error) {
	entries, err := defs.ReadDir("defs")
	if err != nil {
		return nil, err
	}
	r := &Registry{byType: map[string]*Template{}}
	for _, e := range entries {
		b, err := defs.ReadFile("defs/" + e.Name())
		if err != nil {
			return nil, err
		}
		var t Template
		if err := yaml.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if t.Type == "" || len(t.Sections) == 0 {
			return nil, fmt.Errorf("%s: invalid template", e.Name())
		}
		r.byType[t.Type] = &t
	}
	return r, nil
}

func (r *Registry) Get(t string) (*Template, bool) { x, ok := r.byType[t]; return x, ok }

func (r *Registry) List() []*Template {
	out := make([]*Template, 0, len(r.byType))
	for _, t := range r.byType {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

func (t *Template) Section(key string) (*Section, bool) {
	for i := range t.Sections {
		if t.Sections[i].Key == key {
			return &t.Sections[i], true
		}
	}
	return nil, false
}

func (s *Section) Question(key string) (*Question, bool) {
	for i := range s.Questions {
		if s.Questions[i].Key == key {
			return &s.Questions[i], true
		}
	}
	return nil, false
}
