# Spreadsheet XLSX Tooling

Use this reference for local Excel `.xlsx` and CSV operations through the sandbox `office` CLI.

## Commands

```bash
office create-xlsx --spec spec.json --out model.xlsx
office inspect-xlsx model.xlsx --out inspect.json
office edit-xlsx input.xlsx --ops ops.json --out output.xlsx
office inspect-csv input.csv --out inspect.json
office validate output.xlsx --expect expect.json
office render output.xlsx --out-dir rendered
```

## Required Flow

For an existing workbook:

1. Run `office inspect-xlsx input.xlsx --out inspect.json`.
2. Identify sheets, formulas, headers, and sample rows before editing.
3. Write an ops JSON file.
4. Run `office edit-xlsx`.
5. Inspect the output workbook.
6. Validate key sheets, formulas, text, and required cells.
7. Render if the workbook is a formal deliverable or print layout matters.

For a new workbook:

1. Write a spec JSON file.
2. Separate input assumptions, calculation rows, and summary outputs when the workbook is a model.
3. Use formulas instead of hard-coded calculated results.
4. Run `office create-xlsx`.
5. Inspect formulas and sheet names.
6. Validate key formulas and labels.

## Current Capabilities

Use the current `create-xlsx` spec for:

- multi-sheet workbooks
- rows and formulas
- header styling
- column widths
- Excel tables
- basic bar and line charts

Use the current `edit-xlsx` ops for:

- `set_cell`
- `set_formula`
- `append_row`
- `add_sheet`
- `set_width`

Use `inspect-csv` to understand delimited input before converting or modeling it.

## Known Gaps

Treat these as sandbox gaps unless later commands prove support exists:

- freeze panes
- filters and slicers
- pivot tables
- data validation dropdowns
- conditional formatting
- advanced charts
- formula recalculation cache guarantees
- workbook protection
- named ranges
- macro-enabled workbook editing

For financial, budget, pricing, inventory, or KPI files, validation must include formulas, not just visible labels.
