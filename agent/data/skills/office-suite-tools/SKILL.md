---
name: office-suite-tools
description: Use when the agent needs to operate local office artifacts in the sandbox to create, inspect, edit, validate, render, or convert Word .docx, Excel .xlsx/.csv, PowerPoint .pptx, or PDF files. Trigger for file operations and deliverable production that require editable office files, spreadsheet formulas, presentation decks, PDFs, layout verification, or round-trip validation. This skill is about using sandbox tools correctly, not deciding the content structure of a paper, report, quotation model, or presentation narrative.
---

# Office Suite Tools

Use the sandbox runtime as the source of truth for office file operations. Do not hand-roll ad hoc Python or shell scripts for supported operations when the sandbox CLI can do the job.

## Core Workflow

1. Check available commands with `office capabilities` when uncertain.
2. Inspect existing input files before editing or summarizing them.
3. Create files from a JSON spec, then inspect the output.
4. Edit files from a JSON ops file, then inspect the output.
5. Validate required content, formulas, sheets, tables, or layout-critical facts with `office validate` or the format-specific validator.
6. Render formal deliverables, print-sensitive layouts, or user-facing outputs before claiming completion.
7. Report unsupported requirements as sandbox gaps instead of silently bypassing the runtime.

## Tool Routing

- Word `.docx`: read `references/word-docx.md`.
- Excel `.xlsx`, `.xlsm`, `.csv`, `.tsv`: read `references/spreadsheet-xlsx.md`.
- PowerPoint `.pptx`: read `references/presentation-pptx.md`.
- PDF: read `references/pdf.md`.
- Validation expectations and completion bar: read `references/validation.md`.

## Current Sandbox Commands

The current installed sandbox command is `office`.

Supported now:

- `office capabilities`
- `office self-test`
- `office create-docx --spec spec.json --out output.docx`
- `office inspect-docx input.docx [--out inspect.json]`
- `office edit-docx input.docx --ops ops.json --out output.docx`
- `office create-xlsx --spec spec.json --out output.xlsx`
- `office inspect-xlsx input.xlsx [--out inspect.json]`
- `office edit-xlsx input.xlsx --ops ops.json --out output.xlsx`
- `office inspect-csv input.csv [--out inspect.json]`
- `office validate input.docx|input.xlsx --expect expect.json`
- `office render input.docx|input.xlsx --out-dir rendered`

Planned sandbox commands:

- `office create-pptx`, `office inspect-pptx`, `office edit-pptx`, `office validate-pptx`, `office render-pptx`
- `pdf capabilities`, `pdf inspect`, `pdf render`, `pdf extract-text`, `pdf extract-tables`, `pdf merge`, `pdf split`, `pdf watermark`, `pdf compress`, `pdf ocr`, `pdf validate`

Use planned commands only after confirming they exist in the sandbox.

## Hard Rules

- Never treat file creation success as enough. Always inspect.
- Never treat XML/text extraction as enough for layout-sensitive files. Render when visual layout matters.
- Preserve editability unless the user explicitly wants a final PDF only.
- Keep specs and ops files in the working directory so the operation is reproducible.
- If a user asks for content quality or structure, combine this tool skill with a content-organization skill. This skill only covers file operations.
