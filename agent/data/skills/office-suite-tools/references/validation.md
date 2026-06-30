# Office Validation

Use this reference to decide when an office task is actually complete.

## Minimum Validation

Every office operation must meet all of these:

- command exits successfully
- output file exists and is non-empty
- format-specific inspect command succeeds
- important expected content is present

## Formal Deliverable Validation

For user-facing, formal, print-sensitive, or layout-sensitive outputs, also render the file and inspect the rendered result.

Use:

```bash
office render output.docx --out-dir rendered
office render output.xlsx --out-dir rendered
```

For planned PPTX and PDF tools, use render commands only after confirming they exist.

## DOCX Expectations

Validate:

- title and required headings
- required paragraphs or phrases
- table count when tables matter
- required table text
- rendered PDF when layout matters

## XLSX Expectations

Validate:

- required sheet names
- required labels
- required formulas
- key cell values when available
- chart or table presence when supported
- rendered PDF when presentation or print layout matters

For formulas, checking visible values is not enough. Confirm the formula string exists.

## PPTX Expectations

Validate after PPTX support is installed:

- slide count
- required slide titles
- required visible text
- speaker notes when requested
- rendered thumbnails or PDF

## PDF Expectations

Validate after PDF support is installed:

- page count
- required text
- expected page operations such as merge or split results
- rendered page images for visual correctness

## Completion Language

When reporting completion, include:

- created or modified file path
- inspect result summary
- validation result summary
- render result if performed
- any unsupported operation or fallback used
