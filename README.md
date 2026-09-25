# distill

`distill` converts legacy management-system markdown into probe-oriented gemara `Policy` artifacts.

```bash
go get github.com/Formulary-Labs/distill
```

## What it does

Given a directory of legacy ISMS/AIMS/CSMS prose (charters, policies, roles, communication plans), `distill` classifies each file and emits gemara YAML:

- **Policy** — management-system body content (Layer 3). Bound to Annex SL clauses via `metadata.applicability-groups` (`clause-5.2`, etc.).
- **Packet-routed** — SoA, risk registers, impact worksheets, evidence registries are **not** emitted as Policy; they are recorded in `conversion-report.json` for the assessment-packet path (`formula` / `specimen`).

Structure and classification are deterministic. Agent assistance is out of band for v1 — any ambiguous field stays as `[DATA NEEDED]` inside the Policy contacts block.

## Usage

```sh
distill --input docs/legacy-isms --out data/iso27001/gemara --program iso27001

# Optional hints
distill --input docs/legacy --out data/p/gemara --hints hints.json

# Then validate
probe --program iso27001 data/iso27001/gemara/*.yaml
```

### Hints JSON

```json
{
  "default_author": "compliance-team",
  "by_filename": {
    "communication-plan.md": { "type": "Policy", "clause": "7.4" },
    "soa.xlsx.md": { "type": "packet", "packet_kind": "soa" }
  },
  "by_title": {
    "roles": { "type": "Policy", "clause": "5.3" }
  }
}
```

## Outputs

| File | Purpose |
|---|---|
| `<out>/*.yaml` | gemara `Policy` artifacts (verbatim markdown in `metadata.description`) |
| `<out>/conversion-report.json` | source → type → layer → clause provenance |

## Pipeline

```text
distill → probe → compound --assemble-plan → challenge → exhibit
         ↘ formula / titer / specimen  (assessment packet)
```

## Part of Formulary

`distill` is part of the [Formulary](https://github.com/Formulary-Labs) compliance micro-tool ecosystem. Compose with `probe`, `compound --assemble-plan`, and packet tools (`formula`, `specimen`, `titer`). Keep `appraise` for auditor narratives that may include risk/eval content.

## License

Apache License 2.0
