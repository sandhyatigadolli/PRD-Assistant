package tools

import (
	"os"
	"testing"
)

const sample = "# Acme Clinic\n\n## Overview\n\n**Problem statement** [P0] [V1]\n\nClinics lose 20% of appointments – “no-shows”.\n\n### Persona 1\n\n- First bullet with **bold** text that is long enough to wrap onto a second line so we can see the hanging indent behaves properly\n- Second\n1. One\n2. Two\n"

func TestExportSample(t *testing.T) {
	for _, f := range []string{"pdf", "docx"} {
		d, _, ext, err := Exporter{}.Export(f, sample)
		if err != nil || len(d) < 200 {
			t.Fatalf("%s: %v", f, err)
		}
		if dir := os.Getenv("PRD_SAMPLE_DIR"); dir != "" {
			_ = os.WriteFile(dir+"/sample."+ext, d, 0o644)
		}
	}
}
