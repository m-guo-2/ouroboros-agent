---
name: visual-prompt-designer
description: Use when the user wants to generate an expressive image, meme, sticker, poster, visual summary, lightweight report poster, or social sharing graphic. Convert the user's intent into a concise, high-quality prompt for image_generate / Doubao Seedream, while avoiding use for strict numeric reports that require exact text and data.
metadata:
  short-description: Design prompts for expressive image generation
---

# Visual Prompt Designer

Use this skill to convert a user's visual intent into a prompt that can be passed to `image_generate`.

This skill does not generate the image by itself. It prepares the image prompt and recommended size.

## When to use

- Memes, stickers, avatars, reaction images
- Posters, covers, social sharing graphics
- Visual summaries, lightweight report posters, campaign recap images
- Product mood images, concept art, illustrations, comic panels

## When not to use

Do not use generated images as the source of truth for strict numbers, tables, rankings, dates, legal wording, finance, audit, or KPI dashboards.

For those cases, prefer deterministic `data_report` / `render_card`. If the user still wants an expressive image, make it clear in the prompt that dense tables and exact numbers should be avoided.

## Workflow

1. Identify the image type: meme, poster, report poster, cover, illustration, product visual, etc.
2. Extract the subject: who or what is the center of the image.
3. Extract the emotion/action: happy, confused, shocked, celebrating, under pressure, moving fast, etc.
4. Choose composition: square sticker, vertical mobile poster, horizontal cover, centered object, grid layout.
5. Handle text carefully:
   - If text is needed, keep it short and write it exactly.
   - If exact text is not essential, prefer `no text`.
   - Avoid long tables, tiny labels, and dense numeric blocks.
6. Return the `image_generate` arguments, then call `image_generate`.

Ask one concise question only when a missing choice would materially change the image. Otherwise choose a reasonable direction.

## Prompt Shape

A good prompt includes:

- Subject: main subject and supporting elements
- Scene: environment or background
- Composition: framing, layout, aspect ratio
- Style: medium, color, lighting, material, visual tone
- Text: exact short text, or `no text`
- Constraints: no watermark, no extra text, no logo copying, no incorrect numbers

## Meme / Sticker Template

```text
Create a square chat sticker / meme image.
Subject: <subject>.
Emotion: <emotion>.
Action: <action>.
Style: bold clean sticker illustration, expressive face, thick outline, high contrast, simple background.
Text: "<exact short text>".
Keep the text large, centered, and exactly as written.
No watermark, no extra text.
```

## Report Poster Template

```text
Create a polished social media report poster.
Theme: <theme>.
Key visual: <main visual metaphor>.
Layout: strong headline area, 3-5 visual blocks, clear hierarchy, mobile-friendly vertical composition.
Style: modern editorial design, crisp typography, premium but energetic colors.
Text: only include these exact short labels: <labels>.
Avoid tiny tables and dense numbers.
No watermark, no extra text.
```

## Visual Summary Template

```text
Create an infographic-style visual summary.
Topic: <topic>.
Use symbolic visual blocks rather than precise tables.
Composition: clean grid, strong central title, a few simple icon-like illustrations, spacious layout.
Style: modern flat editorial illustration, readable hierarchy.
Text: <short text requirement or no text>.
No watermark, no extra text.
```

## Output

Before calling `image_generate`, prepare arguments like:

```json
{
  "prompt": "...",
  "size": "2K"
}
```

Then call `image_generate` with those arguments.
