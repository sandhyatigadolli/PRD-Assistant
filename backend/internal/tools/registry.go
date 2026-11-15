// Package tools holds the capabilities the agent can use. The Registry makes
// them discoverable (GET /api/tools) the same way an agent framework would.
package tools

import "prd-assistant/internal/llm"

type Tool interface {
	Name() string
	Description() string
}

type Registry struct {
	Validator Validator
	Differ    Differ
	Exporter  Exporter
	all       []Tool
}

func NewRegistry(h *llm.Holder) *Registry {
	r := &Registry{Validator: Validator{Holder: h}}
	r.all = []Tool{r.Validator, r.Differ, r.Exporter}
	return r
}

func (r *Registry) List() []Tool { return r.all }
