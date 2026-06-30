# PowerPoint PPTX Tooling

Use this reference for local PowerPoint `.pptx` operations. The current sandbox does not yet expose PPTX commands. Confirm capability before using planned commands.

## Planned Commands

```bash
office create-pptx --spec spec.json --out deck.pptx
office inspect-pptx deck.pptx --out inspect.json
office edit-pptx input.pptx --ops ops.json --out output.pptx
office validate-pptx output.pptx --expect expect.json
office render-pptx output.pptx --out-dir rendered
```

## Required Flow

For an existing deck:

1. Confirm PPTX support exists with `office capabilities`.
2. Inspect slide count, titles, text blocks, images, charts, and speaker notes.
3. Write an ops JSON file.
4. Edit through the sandbox command.
5. Inspect the output.
6. Render thumbnails or PDF for visual verification.

For a new deck:

1. Write a spec JSON file with theme, layouts, and slides.
2. Create the deck through the sandbox command.
3. Inspect slide count and page content.
4. Render thumbnails or PDF before final delivery.

## Tool Responsibilities

The PPTX tool skill covers:

- creating slides
- applying layouts
- inserting text boxes, images, tables, and charts
- adding speaker notes
- editing existing slide content
- exporting PDF or slide thumbnails
- validating slide count, titles, and required text

It does not decide the narrative, storyline, or message hierarchy. Use a content-organization skill for that.

## Minimum Capability To Implement

The sandbox PPTX tool should support:

- title slides
- section divider slides
- text content slides
- image slides
- table slides
- chart slides
- closing slides
- speaker notes
- PDF rendering
- thumbnail rendering
- inspect output with slide titles and text
