# Word DOCX Tooling

Use this reference for local Word `.docx` operations through the sandbox `office` CLI.

## Commands

```bash
office create-docx --spec spec.json --out result.docx
office inspect-docx result.docx --out inspect.json
office edit-docx input.docx --ops ops.json --out output.docx
office validate output.docx --expect expect.json
office render output.docx --out-dir rendered
```

## Required Flow

For an existing document:

1. Run `office inspect-docx input.docx --out inspect.json`.
2. Use the inspection result to decide the edit plan.
3. Write an ops JSON file.
4. Run `office edit-docx`.
5. Inspect the edited output.
6. Validate important expected text, table counts, or headings.
7. Render when page layout, printing, or final presentation matters.

For a new document:

1. Write a spec JSON file.
2. Run `office create-docx`.
3. Inspect the output.
4. Validate required content.
5. Render if the document is formal or layout-sensitive.

## Current Capabilities

Use the current `create-docx` spec for:

- title and subtitle
- page margins
- base styles
- headings
- paragraphs
- bullet lists
- tables with header fill

Use the current `edit-docx` ops for:

- `replace_text`
- `append_paragraph`
- `set_table_cell`

## Known Gaps

Treat these as sandbox gaps unless later commands prove support exists:

- automatic table of contents
- page headers and footers
- page numbers
- footnotes and endnotes
- images
- comments
- tracked changes
- complex list numbering
- section breaks and columns
- strict template style preservation

When a task needs one of these, say the sandbox tool needs extension or use a clearly labeled fallback.
