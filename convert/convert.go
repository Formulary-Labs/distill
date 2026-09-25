// Package convert implements deterministic legacy markdown → gemara Policy conversion.
package convert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Hints maps filename patterns or titles to artifact routing.
type Hints struct {
	// ByFilename maps basename (or glob-ish contains) → clause or route.
	ByFilename map[string]Hint `json:"by_filename" yaml:"by_filename"`
	// ByTitle maps title substring (case-insensitive) → hint.
	ByTitle map[string]Hint `json:"by_title" yaml:"by_title"`
	// DefaultAuthor used in emitted Policy metadata.
	DefaultAuthor string `json:"default_author,omitempty"`
	// Organization used in emitted Policy titles when needed.
	Organization string `json:"organization,omitempty"`
}

// Hint routes a source document.
type Hint struct {
	// Type is Policy (default) or packet (SoA/risk/impact → not emitted as Policy).
	Type string `json:"type"` // "Policy" | "packet"
	// Clause is Annex SL clause slot, e.g. "5.2", "5.3", "7.4".
	Clause string `json:"clause,omitempty"`
	// PacketKind when Type=packet: soa | risk | impact | evidence.
	PacketKind string `json:"packet_kind,omitempty"`
}

// ConversionReport is written alongside emitted gemara files.
type ConversionReport struct {
	Program       string             `json:"program"`
	InputDir      string             `json:"input_dir"`
	OutputDir     string             `json:"output_dir"`
	Entries       []ConversionEntry  `json:"entries"`
	Emitted       []string           `json:"emitted"`
	Gaps          []string           `json:"gaps,omitempty"`
	PacketRouted  []string           `json:"packet_routed,omitempty"`
	AgentAssisted []string           `json:"agent_assisted,omitempty"` // always empty in v1 (deterministic)
}

// ConversionEntry records one source → artifact mapping.
type ConversionEntry struct {
	Source       string `json:"source"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	Layer        string `json:"layer"` // gemara layer label for Policy = "3"
	Clause       string `json:"clause,omitempty"`
	Output       string `json:"output,omitempty"`
	PacketKind   string `json:"packet_kind,omitempty"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// Result of a conversion run.
type Result struct {
	Report ConversionReport
}

var (
	headingRE = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	packetRE  = regexp.MustCompile(`(?i)(soa|statement of applicability|risk register|risk treatment|impact (assessment|worksheet)|evidence registry)`)
	clauseRE  = regexp.MustCompile(`(?i)\b(?:clause\s*)?(\d+(?:\.\d+)?)\b`)
)

// LoadHints reads an optional hints JSON file.
func LoadHints(path string) (*Hints, error) {
	if path == "" {
		return &Hints{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading hints %q: %w", path, err)
	}
	var h Hints
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parsing hints %q: %w", path, err)
	}
	if h.ByFilename == nil {
		h.ByFilename = map[string]Hint{}
	}
	if h.ByTitle == nil {
		h.ByTitle = map[string]Hint{}
	}
	return &h, nil
}

// Run converts markdown files under inputDir into gemara Policy YAML under outputDir.
func Run(program, inputDir, outputDir string, hints *Hints, dryRun bool) (*Result, error) {
	if hints == nil {
		hints = &Hints{}
	}
	if program == "" {
		program = "program"
	}

	entries, err := listMarkdown(inputDir)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no markdown files found in %q", inputDir)
	}

	if !dryRun {
		if err := os.MkdirAll(outputDir, 0o750); err != nil {
			return nil, fmt.Errorf("creating output dir: %w", err)
		}
	}

	report := ConversionReport{
		Program:   program,
		InputDir:  inputDir,
		OutputDir: outputDir,
	}

	for _, src := range entries {
		body, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("reading %q: %w", src, err)
		}
		title := extractTitle(string(body), filepath.Base(src))
		hint := resolveHint(filepath.Base(src), title, string(body), hints)

		entry := ConversionEntry{
			Source: src,
			Title:  title,
			Type:   hint.Type,
			Clause: hint.Clause,
		}

		if hint.Type == "packet" {
			entry.Layer = "packet"
			entry.PacketKind = hint.PacketKind
			entry.SkippedReason = "routed to assessment packet — not emitted as Policy"
			report.PacketRouted = append(report.PacketRouted, src)
			report.Entries = append(report.Entries, entry)
			continue
		}

		entry.Type = "Policy"
		entry.Layer = "3"
		if entry.Clause == "" {
			entry.Clause = inferClause(title, string(body))
		}

		id := policyID(program, entry.Clause, title, src)
		outName := id + ".yaml"
		outPath := filepath.Join(outputDir, outName)
		entry.Output = outPath

		yamlBody := emitPolicyYAML(id, title, string(body), entry.Clause, hints)

		if !dryRun {
			if err := os.WriteFile(outPath, []byte(yamlBody), 0o600); err != nil {
				return nil, fmt.Errorf("writing %q: %w", outPath, err)
			}
		}
		report.Emitted = append(report.Emitted, outPath)
		report.Entries = append(report.Entries, entry)
	}

	if len(report.Emitted) == 0 && len(report.PacketRouted) > 0 {
		report.Gaps = append(report.Gaps, "all sources routed to packet — no Policy artifacts emitted")
	}
	for _, e := range report.Entries {
		if e.Type == "Policy" && e.Clause == "" {
			report.Gaps = append(report.Gaps, fmt.Sprintf("%s: no clause binding inferred — defaulted to unbound Policy", e.Source))
		}
	}

	return &Result{Report: report}, nil
}

func listMarkdown(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

func extractTitle(body, basename string) string {
	if m := headingRE.FindStringSubmatch(body); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	base := strings.TrimSuffix(basename, filepath.Ext(basename))
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.ReplaceAll(base, "_", " ")
	return strings.TrimSpace(base)
}

func resolveHint(basename, title, body string, hints *Hints) Hint {
	// Explicit filename hint.
	for k, h := range hints.ByFilename {
		if strings.EqualFold(basename, k) || strings.Contains(strings.ToLower(basename), strings.ToLower(k)) {
			return normalizeHint(h)
		}
	}
	for k, h := range hints.ByTitle {
		if strings.Contains(strings.ToLower(title), strings.ToLower(k)) {
			return normalizeHint(h)
		}
	}
	// Deterministic packet detection from content/title.
	blob := title + "\n" + body
	if packetRE.MatchString(blob) {
		kind := "evidence"
		switch {
		case regexp.MustCompile(`(?i)soa|statement of applicability`).MatchString(blob):
			kind = "soa"
		case regexp.MustCompile(`(?i)risk`).MatchString(blob):
			kind = "risk"
		case regexp.MustCompile(`(?i)impact`).MatchString(blob):
			kind = "impact"
		}
		return Hint{Type: "packet", PacketKind: kind}
	}
	return Hint{Type: "Policy", Clause: inferClause(title, body)}
}

func normalizeHint(h Hint) Hint {
	if h.Type == "" {
		h.Type = "Policy"
	}
	if strings.EqualFold(h.Type, "packet") {
		h.Type = "packet"
	} else {
		h.Type = "Policy"
	}
	return h
}

func inferClause(title, body string) string {
	blob := strings.ToLower(title + " " + body[:min(500, len(body))])
	switch {
	case strings.Contains(blob, "communication"):
		return "7.4"
	case strings.Contains(blob, "role") || strings.Contains(blob, "raci") || strings.Contains(blob, "responsib"):
		return "5.3"
	case strings.Contains(blob, "scope") || strings.Contains(blob, "charter") || strings.Contains(blob, "context"):
		return "4"
	case strings.Contains(blob, "policy"):
		return "5.2"
	case strings.Contains(blob, "audit"):
		return "9.2"
	case strings.Contains(blob, "improvement") || strings.Contains(blob, "corrective"):
		return "10"
	}
	if m := clauseRE.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	return "5.2" // default management-system body → policy clause
}

func policyID(program, clause, title, src string) string {
	base := slugify(title)
	if base == "" {
		base = slugify(filepath.Base(src))
	}
	if clause != "" {
		return fmt.Sprintf("%s-clause-%s-%s", slugify(program), strings.ReplaceAll(clause, ".", "-"), base)
	}
	sum := sha256.Sum256([]byte(src))
	return fmt.Sprintf("%s-%s-%x", slugify(program), base, sum[:4])
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = out[:48]
		out = strings.Trim(out, "-")
	}
	return out
}

func emitPolicyYAML(id, title, body, clause string, hints *Hints) string {
	author := "distill"
	if hints != nil && hints.DefaultAuthor != "" {
		author = hints.DefaultAuthor
	}
	// Verbatim carrier: metadata.description holds full markdown body.
	desc := strings.TrimSpace(body)
	desc = strings.ReplaceAll(desc, "\t", "  ")

	var sb strings.Builder
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  id: %q\n", id))
	sb.WriteString("  type: Policy\n")
	sb.WriteString("  gemara-version: \"1.0.0\"\n")
	sb.WriteString("  description: |\n")
	for _, line := range strings.Split(desc, "\n") {
		sb.WriteString("    " + line + "\n")
	}
	sb.WriteString("  author:\n")
	sb.WriteString(fmt.Sprintf("    id: %q\n", slugify(author)))
	sb.WriteString(fmt.Sprintf("    name: %q\n", author))
	sb.WriteString("    type: Human\n")
	if clause != "" {
		sb.WriteString("  applicability-groups:\n")
		sb.WriteString(fmt.Sprintf("    - id: %q\n", "clause-"+clause))
		sb.WriteString(fmt.Sprintf("      title: %q\n", "Annex SL clause "+clause))
	}
	sb.WriteString(fmt.Sprintf("title: %q\n", title))
	sb.WriteString("contacts:\n")
	sb.WriteString("  responsible:\n")
	sb.WriteString("    - name: \"[DATA NEEDED: responsible contact]\"\n")
	sb.WriteString("  accountable:\n")
	sb.WriteString("    - name: \"[DATA NEEDED: accountable contact]\"\n")
	sb.WriteString("scope:\n")
	sb.WriteString("  in: {}\n")
	sb.WriteString("  out: {}\n")
	sb.WriteString("adherence: {}\n")
	sb.WriteString("imports: {}\n")
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
