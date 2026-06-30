---
name: gpt-image-2-newapi
description: Use when the user asks to draw, create, generate, or transform an image, including text-to-image and image-reference generation from one or more input images. This skill uses the user's New API Responses gateway with the hosted image_generation tool and saves both prompt archives and generated image files.
metadata:
  short-description: Generate images via New API Responses image_generation
---

# GPT Image 2 New API

Use this skill for image generation requests that should go through the user's New API gateway, including text-to-image and image-reference generation:

- Verified moli-prod server-side endpoint: `POST http://127.0.0.1:2026/v1/responses`
- Main model default: `gpt-5.4-mini`
- Image tool model default: `gpt-image-2`
- Tool type: `image_generation`
- Current moli-prod Responses forwarding requires `stream: true`; non-streaming requests return `400 Stream must be set to true`.

Do not hard-code tokens in prompts, scripts, or generated artifacts. Read them from local env files or process env.

## Quick Start

From the skill directory:

```bash
export NEWAPI_BASE_URL=http://127.0.0.1:2026
export MOLI_TOKEN="$(scripts/get-moli-token-from-server.sh)"

node scripts/generate-response-image.js \
  --prompt "画一个红色方块图标，白色背景，简洁扁平风格" \
  --size 1024x1024 \
  --format png
```

Use a local image as reference:

```bash
node scripts/generate-response-image.js \
  --prompt "参考这张图，生成一个同风格的红色方块图标，白色背景" \
  --image /path/to/reference.png \
  --size 1024x1024 \
  --format png
```

The script saves and uploads:

- prompt archive with reference image paths/URLs: `newapi-image-output/prompt/<slug>-<timestamp>.md`
- final image: `newapi-image-output/image/<slug>-<timestamp>.png`
- optional partial images: `newapi-image-output/image/<slug>-<timestamp>-partial-<n>.png`
- OSS object for each final image when OSS config is available; the script prints `oss_uri=oss://bucket/key` and `oss_url=http(s)://...`

## Production Verification

Before asking a user to retry after a deployment or sync change, verify the server-side cached skill:

```bash
cd /opt/moli/data/skills/gpt-image-2-newapi
scripts/verify-server-skill.sh
```

The check must print:

- `version_ok=yes version=2026-06-04-final-result-required`
- `api_key_present=yes`
- `oss_config=present`
- `partial_policy_ok=yes`

This check proves that the running server cache has the intended script version, NewAPI credentials are available, OSS config is readable, and `partial_image` is not treated as a final result.

## Configuration

The script reads values in this order:

1. CLI flags
2. process env
3. `.env` in the current working directory
4. `.gateway.env` in the current working directory
5. `~/.gateway.env`

Supported variables:

- `NEWAPI_BASE_URL` or `OPENAI_BASE_URL`: gateway base URL, with or without trailing `/v1`
- `MOLI_TOKEN`, `NEWAPI_API_KEY`, or `OPENAI_API_KEY`: bearer token
- `NEWAPI_MAIN_MODEL`: defaults to `gpt-5.4-mini`
- `NEWAPI_IMAGE_MODEL`: defaults to `gpt-image-2`
- `OSS_ENDPOINT`, `OSS_BUCKET`, `OSS_ACCESS_KEY`, `OSS_SECRET_KEY`, `OSS_REGION`, `OSS_PREFIX`, `OSS_USE_SSL`, `OSS_PUBLIC_BASE_URL`: optional OSS upload config

OSS upload behavior:

- The script first reads OSS config from env.
- If env is not set and `/opt/moli/conf/agent.yaml` exists, it reads the existing `oss:` section used by the agent service.
- `OSS_PREFIX` defaults to `generated/images`.
- Use `--no-oss-upload` only when you need a local file without uploading.

For the current moli-prod deployment:

- Container: `new-api`
- Docker image: `calciumion/new-api:latest`
- Runtime port: host `2026` -> container `3000`
- Server-side calls on moli-prod should use `http://127.0.0.1:2026` to avoid routing through the public address.
- Valid token name in DB: `moli`
- Active image-capable channel: `Codex Forwarder`
- Do not use `http://115.190.14.209/` for this API call; it currently serves a different nginx site.
- Do not use `new-api.115.190.14.209.nip.io`; it is currently intercepted by Volcengine webblock.

## Workflow

1. Convert the user's request into a compact image prompt. Preserve concrete visual constraints such as subject, style, background, aspect ratio, text, brand, and output format.
2. If the user supplied image files or URLs, pass them with `--image` or `--image-url`; do not describe the image from memory when an actual file can be attached.
3. If the prompt lacks a key visual requirement that materially changes the image, ask one precise question. Otherwise proceed.
4. Run `scripts/generate-response-image.js` through `run_script` with `timeout_seconds: 600`. Image generation can take several minutes. Do not set `async: true`; the agent needs the `oss_url` in the same tool result before replying.
5. Return the `oss_url` first when present, then the local saved image path and, when useful, the revised prompt printed by the API.

When this skill is loaded inside Moli/agent, use `run_script` instead of making a direct HTTP request:

```json
{
  "skill_id": "gpt-image-2-newapi",
  "script": "generate-response-image.js",
  "args": "--prompt \"一个极简蓝色圆形图标，白色背景，无文字\" --size 1024x1024 --format png",
  "timeout_seconds": 600
}
```

Never set `async: true` for this image skill. If `run_script` returns without an `oss_url=...` line, treat it as failed and do not tell the user the image is ready.
`partial_image=...` is not a successful result. Only upload and return a final image when the script prints `image=...` and `oss_url=...`.

Do not use `/v1/chat/completions` for image generation. The image-capable route is `/v1/responses` with `stream: true`, and the script already uses that route.

## CLI Flags

- `--prompt <text>`: required unless `--promptfile` is provided
- `--promptfile <path>`: read the prompt from a file
- `--image <path>`: add a local reference image; can be repeated. Supports png, jpg, jpeg, webp, and gif.
- `--image-url <url>`: add a remote reference image URL; can be repeated.
- `--base-url <url>`: overrides `NEWAPI_BASE_URL`
- `--api-key <token>`: overrides env token; avoid using this in shared logs
- `--model <model>`: main Responses model
- `--image-model <model>`: image generation tool model
- `--size <size>`: default `1024x1024`
- `--format <png|jpeg|webp>`: default `png`
- `--quality <low|medium|high|auto>`: optional
- `--background <opaque|transparent|auto>`: optional
- `--partial-images <0|1|2|3>`: default `0`
- `--output-dir <path>`: default `newapi-image-output`
- `--no-oss-upload`: disable OSS upload for local-only runs
- `--agent-config <path>`: YAML config path for reading the `oss:` section; defaults to `/opt/moli/conf/agent.yaml` when present
- `--oss-prefix <prefix>`: override OSS key prefix; default `generated/images`
- `--no-stream`: use only on gateways that support non-streaming Responses; moli-prod currently does not.

## Notes

- The Responses API returns final generated image data on `image_generation_call.result`.
- In streaming mode, partial images may arrive as `response.image_generation_call.partial_image` events.
- If the gateway does not support `gpt-image-2` or `gpt-5.4-mini`, retry only after confirming the gateway's supported model names.
- Verified on moli-prod: the sample red-square prompt returned a final PNG and revised prompt.
