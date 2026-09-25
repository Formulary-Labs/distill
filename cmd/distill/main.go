// distill converts legacy markdown compliance docs into gemara Policy artifacts.
//
// Usage:
//
//	distill --input <docs-dir> --out <gemara-dir> [--program slug] [--hints hints.json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Formulary-Labs/distill/convert"
	"github.com/Formulary-Labs/substrate/exit"
	"github.com/Formulary-Labs/substrate/provenance"
)

const version = "0.1.0"

func main() {
	var (
		inputFlag   = flag.String("input", "", "Directory of legacy markdown docs (required)")
		outFlag     = flag.String("out", "", "Output directory for gemara artifacts (required)")
		programFlag = flag.String("program", "", "Program slug")
		hintsFlag   = flag.String("hints", "", "Optional hints JSON (filename/title → type/clause)")
		reportFlag  = flag.String("report", "", "Conversion report JSON path (default: <out>/conversion-report.json)")
		fmtFlag     = flag.String("format", "json", "Stdout summary format: json (default), md")
		dryRunFlag  = flag.Bool("dry-run", false, "Classify and report without writing artifacts")
		versionFlag = flag.Bool("version", false, "Print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *versionFlag {
		fmt.Printf("distill version %s\n", version)
		os.Exit(exit.OK)
	}

	if *inputFlag == "" || *outFlag == "" {
		fmt.Fprintln(os.Stderr, `{"error": "--input and --out are required", "code": 2}`)
		flag.Usage()
		os.Exit(exit.ToolError)
	}

	hints, err := convert.LoadHints(*hintsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error": %q, "code": 2}`+"\n", err.Error())
		os.Exit(exit.ToolError)
	}

	program := *programFlag
	if program == "" {
		program = filepath.Base(filepath.Clean(*outFlag))
	}

	result, err := convert.Run(program, *inputFlag, *outFlag, hints, *dryRunFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error": %q, "code": 2}`+"\n", err.Error())
		os.Exit(exit.ToolError)
	}

	reportPath := *reportFlag
	if reportPath == "" {
		reportPath = filepath.Join(*outFlag, "conversion-report.json")
	}
	if !*dryRunFlag {
		data, err := json.MarshalIndent(result.Report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error marshaling report: %v\n", err)
			os.Exit(exit.ToolError)
		}
		if err := os.MkdirAll(filepath.Dir(reportPath), 0o750); err != nil {
			fmt.Fprintf(os.Stderr, "error creating report dir: %v\n", err)
			os.Exit(exit.ToolError)
		}
		if err := os.WriteFile(reportPath, data, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "error writing report: %v\n", err)
			os.Exit(exit.ToolError)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", reportPath)
		for _, p := range result.Report.Emitted {
			fmt.Fprintf(os.Stderr, "wrote %s\n", p)
		}
	}

	switch *fmtFlag {
	case "md":
		printMD(result.Report)
	default:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result.Report)
	}

	_ = provenance.Write("logs/provenance.jsonl", provenance.Entry{
		Spec:        "functions/gemara-distill-spec.md",
		Output:      coalesce(*outFlag, "stdout"),
		OutputType:  "other",
		Program:     program,
		Purpose:     fmt.Sprintf("distill: %d sources → %d Policy artifacts", len(result.Report.Entries), len(result.Report.Emitted)),
		Reusability: provenance.Instance,
		QualityGate: provenance.Pass,
		Tool:        "distill",
		ToolVersion: version,
	})
}

func printMD(r convert.ConversionReport) {
	fmt.Printf("# Conversion Report — %s\n\n", r.Program)
	fmt.Printf("**Input:** `%s`  \n**Output:** `%s`  \n\n", r.InputDir, r.OutputDir)
	fmt.Printf("| Source | Type | Clause | Output |\n|---|---|---|---|\n")
	for _, e := range r.Entries {
		out := e.Output
		if out == "" {
			out = e.SkippedReason
		}
		fmt.Printf("| `%s` | %s | %s | `%s` |\n", e.Source, e.Type, e.Clause, out)
	}
	fmt.Println()
	if len(r.Gaps) > 0 {
		fmt.Println("## Gaps")
		for _, g := range r.Gaps {
			fmt.Printf("- %s\n", g)
		}
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func usage() {
	fmt.Fprintln(os.Stderr, `distill — legacy docs → gemara Policy artifacts

Usage:
  distill --input <docs-dir> --out <gemara-dir> [flags]

Flags:
  --input string     Directory of legacy markdown docs (required)
  --out string       Output directory for gemara YAML (required)
  --program string   Program slug
  --hints string     Hints JSON (by_filename / by_title → type/clause)
  --report string    Conversion report path (default: <out>/conversion-report.json)
  --format string    Stdout format: json (default), md
  --dry-run          Classify without writing
  --version          Print version and exit

After distill, validate emitted artifacts:
  probe --program <slug> <out>/*.yaml

Examples:
  distill --input docs/legacy-isms --out data/iso27001/gemara --program iso27001
  distill --input docs/legacy --out data/p/gemara --hints hints.json --dry-run`)
}
