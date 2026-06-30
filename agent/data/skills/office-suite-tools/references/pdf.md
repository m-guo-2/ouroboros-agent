# PDF Tooling

Use this reference for local PDF operations. PDF should have its own sandbox CLI because PDF is both an input artifact and a final deliverable, not just an Office export format.

## Planned Commands

```bash
pdf capabilities
pdf inspect input.pdf --out inspect.json
pdf render input.pdf --out-dir pages
pdf extract-text input.pdf --out text.json
pdf extract-tables input.pdf --out tables.json
pdf merge --inputs a.pdf,b.pdf --out merged.pdf
pdf split input.pdf --pages 1-3 --out-dir parts
pdf watermark input.pdf --text "CONFIDENTIAL" --out output.pdf
pdf compress input.pdf --out compressed.pdf
pdf ocr input.pdf --out searchable.pdf
pdf validate input.pdf --expect expect.json
```

Use these commands only after confirming they exist.

## Required Flow

For reading a PDF:

1. Inspect metadata, page count, text availability, and whether the PDF appears scanned.
2. Extract text only when the file has text.
3. Use OCR for scanned PDFs.
4. Extract tables with page and coordinate context when table structure matters.

For modifying a PDF:

1. Inspect before editing.
2. Apply merge, split, watermark, compression, or OCR through the sandbox CLI.
3. Inspect after editing.
4. Render pages when visual correctness matters.
5. Validate page count and required text.

## Tool Responsibilities

The PDF tool skill covers:

- inspecting page count and metadata
- rendering pages for visual QA
- extracting text
- extracting tables
- OCR
- merging and splitting
- watermarking
- compression
- validation

It does not replace editable source files. If the user needs an editable document, create or edit DOCX/XLSX/PPTX first, then export or render PDF.
