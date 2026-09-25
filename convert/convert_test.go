package convert_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Formulary-Labs/distill/convert"
)

func TestRun_emitsPolicyPerDoc(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	mustWrite(t, filepath.Join(in, "information-security-policy.md"), `# Information Security Policy

This policy establishes controls for protecting information assets.

## Commitments

- Continual improvement
- Satisfy applicable requirements
`)
	mustWrite(t, filepath.Join(in, "roles-and-responsibilities.md"), `# Roles and Responsibilities

RACI for the ISMS program.
`)
	mustWrite(t, filepath.Join(in, "soa.md"), `# Statement of Applicability

Control inclusion/exclusion grid — packet material.
`)

	result, err := convert.Run("iso27001", in, out, nil, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.Report.Emitted) != 2 {
		t.Fatalf("expected 2 Policy emissions, got %d: %v", len(result.Report.Emitted), result.Report.Emitted)
	}
	if len(result.Report.PacketRouted) != 1 {
		t.Fatalf("expected 1 packet-routed source, got %d", len(result.Report.PacketRouted))
	}

	found52 := false
	found53 := false
	for _, p := range result.Report.Emitted {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !strings.Contains(s, "type: Policy") {
			t.Errorf("%s missing type: Policy", p)
		}
		if !strings.Contains(s, "gemara-version:") {
			t.Errorf("%s missing gemara-version", p)
		}
		if strings.Contains(s, "clause-5.2") {
			found52 = true
			if !strings.Contains(s, "Protecting information assets") && !strings.Contains(s, "protecting information assets") {
				// body is in description block
				if !strings.Contains(s, "information assets") {
					t.Errorf("policy body not carried in description")
				}
			}
		}
		if strings.Contains(s, "clause-5.3") {
			found53 = true
		}
	}
	if !found52 {
		t.Error("expected a Policy bound to clause 5.2")
	}
	if !found53 {
		t.Error("expected a Policy bound to clause 5.3")
	}
}

func TestRun_hintsOverride(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()
	mustWrite(t, filepath.Join(in, "comms.md"), "# Weekly Sync Notes\n\nStatus update.\n")

	hints := &convert.Hints{
		ByFilename: map[string]convert.Hint{
			"comms.md": {Type: "Policy", Clause: "7.4"},
		},
	}
	result, err := convert.Run("prog", in, out, hints, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Report.Emitted) != 1 {
		t.Fatalf("expected 1 emission, got %d", len(result.Report.Emitted))
	}
	data, _ := os.ReadFile(result.Report.Emitted[0])
	if !strings.Contains(string(data), "clause-7.4") {
		t.Errorf("hints clause not applied: %s", data)
	}
}

func TestLoadHints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hints.json")
	mustWrite(t, path, `{"by_filename":{"x.md":{"type":"Policy","clause":"4"}}}`)
	h, err := convert.LoadHints(path)
	if err != nil {
		t.Fatal(err)
	}
	if h.ByFilename["x.md"].Clause != "4" {
		t.Errorf("unexpected hint: %+v", h.ByFilename["x.md"])
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
