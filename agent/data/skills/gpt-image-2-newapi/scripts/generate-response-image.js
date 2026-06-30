#!/usr/bin/env node

const fs = require("fs");
const crypto = require("crypto");
const os = require("os");
const path = require("path");

const SCRIPT_VERSION = "2026-06-04-final-result-required";
const DEFAULT_MAIN_MODEL = "gpt-5.4-mini";
const DEFAULT_IMAGE_MODEL = "gpt-image-2";

function parseArgs(argv) {
  const args = {};
  const listArgs = new Set(["image", "image-url"]);
  const boolArgs = new Set(["no-stream", "no-oss-upload", "version", "check-config"]);
  for (let i = 0; i < argv.length; i += 1) {
    const key = argv[i];
    if (!key.startsWith("--")) continue;
    const name = key.slice(2);
    if (boolArgs.has(name)) {
      if (name === "no-stream") args.stream = false;
      else args[name] = true;
      continue;
    }
    const value = argv[i + 1];
    if (!value || value.startsWith("--")) {
      throw new Error(`Missing value for --${name}`);
    }
    if (listArgs.has(name)) {
      args[name] = args[name] || [];
      args[name].push(value);
    } else {
      args[name] = value;
    }
    i += 1;
  }
  return args;
}

function parseEnvFile(filePath) {
  if (!fs.existsSync(filePath)) return {};
  const out = {};
  const text = fs.readFileSync(filePath, "utf8");
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const match = line.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
    if (!match) continue;
    let value = match[2].trim();
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    out[match[1]] = value;
  }
  return out;
}

function stripQuotes(value) {
  value = String(value || "").trim();
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return value.slice(1, -1);
  }
  return value;
}

function parseSimpleYamlSection(filePath, sectionName) {
  if (!fs.existsSync(filePath)) return {};
  const out = {};
  const lines = fs.readFileSync(filePath, "utf8").split(/\r?\n/);
  let inSection = false;
  let sectionIndent = 0;
  for (const rawLine of lines) {
    const noComment = rawLine.replace(/\s+#.*$/, "");
    if (!noComment.trim()) continue;
    const indent = noComment.match(/^\s*/)[0].length;
    const trimmed = noComment.trim();
    if (!inSection) {
      if (trimmed === `${sectionName}:`) {
        inSection = true;
        sectionIndent = indent;
      }
      continue;
    }
    if (indent <= sectionIndent) break;
    const match = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*):\s*(.*)$/);
    if (!match) continue;
    out[match[1]] = stripQuotes(match[2]);
  }
  return out;
}

function loadConfig(args) {
  const cwd = process.cwd();
  const fileEnv = Object.assign(
    {},
    parseEnvFile(path.join(os.homedir(), ".gateway.env")),
    parseEnvFile(path.join(cwd, ".gateway.env")),
    parseEnvFile(path.join(cwd, ".env")),
  );
  const env = Object.assign({}, fileEnv, process.env);

  const baseUrl =
    args["base-url"] ||
    env.NEWAPI_BASE_URL ||
    env.OPENAI_BASE_URL;
  const apiKey =
    args["api-key"] ||
    env.MOLI_TOKEN ||
    env.NEWAPI_API_KEY ||
    env.OPENAI_API_KEY;

  if (!baseUrl) {
    throw new Error("Missing NEWAPI_BASE_URL or OPENAI_BASE_URL.");
  }
  if (!apiKey) {
    throw new Error("Missing MOLI_TOKEN, NEWAPI_API_KEY, or OPENAI_API_KEY.");
  }

  return {
    baseUrl: normalizeBaseUrl(baseUrl),
    apiKey,
    mainModel: args.model || env.NEWAPI_MAIN_MODEL || DEFAULT_MAIN_MODEL,
    imageModel: args["image-model"] || env.NEWAPI_IMAGE_MODEL || DEFAULT_IMAGE_MODEL,
    env,
  };
}

function normalizeBaseUrl(baseUrl) {
  let url = baseUrl.replace(/\/+$/, "");
  if (url.endsWith("/v1")) return url;
  return `${url}/v1`;
}

function readPrompt(args) {
  if (args.promptfile) {
    return fs.readFileSync(args.promptfile, "utf8").trim();
  }
  if (args.prompt) return args.prompt.trim();
  throw new Error("Missing --prompt or --promptfile.");
}

function slugify(text) {
  const ascii = text
    .normalize("NFKD")
    .replace(/[^\w\s-]/g, "")
    .trim()
    .replace(/[\s_-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .toLowerCase();
  if (ascii) return ascii.slice(0, 60);
  return "image";
}

function timestamp() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return [
    d.getFullYear(),
    pad(d.getMonth() + 1),
    pad(d.getDate()),
    "-",
    pad(d.getHours()),
    pad(d.getMinutes()),
    pad(d.getSeconds()),
  ].join("");
}

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function writeBase64(filePath, b64) {
  fs.writeFileSync(filePath, Buffer.from(b64, "base64"));
}

function parseBool(value) {
  return ["1", "true", "yes", "on"].includes(String(value || "").trim().toLowerCase());
}

function normalizeOssEndpoint(endpoint, useSSL) {
  let raw = String(endpoint || "").trim().replace(/\/+$/, "");
  if (!raw) return { endpoint: "", useSSL };
  if (raw.startsWith("http://") || raw.startsWith("https://")) {
    const parsed = new URL(raw);
    raw = parsed.host;
    useSSL = parsed.protocol === "https:";
  }
  return { endpoint: raw, useSSL };
}

function loadOssConfig(args, env) {
  if (args["no-oss-upload"]) return null;

  const yamlPath =
    args["agent-config"] ||
    env.AGENT_CONFIG ||
    env.MOLI_AGENT_CONFIG ||
    (fs.existsSync("/opt/moli/conf/agent.yaml") ? "/opt/moli/conf/agent.yaml" : "");
  const yamlOss = yamlPath ? parseSimpleYamlSection(yamlPath, "oss") : {};

  const endpointValue = args["oss-endpoint"] || env.OSS_ENDPOINT || yamlOss.endpoint || "";
  const normalized = normalizeOssEndpoint(
    endpointValue,
    parseBool(args["oss-use-ssl"] || env.OSS_USE_SSL || yamlOss.use_ssl),
  );

  const cfg = {
    endpoint: normalized.endpoint,
    bucket: args["oss-bucket"] || env.OSS_BUCKET || yamlOss.bucket || "",
    accessKey: args["oss-access-key"] || env.OSS_ACCESS_KEY || yamlOss.access_key || "",
    secretKey: args["oss-secret-key"] || env.OSS_SECRET_KEY || yamlOss.secret_key || "",
    region: args["oss-region"] || env.OSS_REGION || yamlOss.region || "us-east-1",
    prefix: args["oss-prefix"] || env.OSS_PREFIX || yamlOss.prefix || "generated/images",
    useSSL: normalized.useSSL,
    publicBaseUrl: args["oss-public-base-url"] || env.OSS_PUBLIC_BASE_URL || yamlOss.public_base_url || "",
  };

  const hasAny = Boolean(cfg.endpoint || cfg.bucket || cfg.accessKey || cfg.secretKey);
  if (!hasAny) return null;

  const missing = [];
  for (const [key, value] of Object.entries({
    endpoint: cfg.endpoint,
    bucket: cfg.bucket,
    accessKey: cfg.accessKey,
    secretKey: cfg.secretKey,
  })) {
    if (!String(value || "").trim()) missing.push(key);
  }
  if (missing.length) {
    throw new Error(`Incomplete OSS config; missing ${missing.join(", ")}.`);
  }
  return cfg;
}

function printConfigCheck(cfg, ossCfg) {
  console.log(`script_version=${SCRIPT_VERSION}`);
  console.log(`base_url=${cfg.baseUrl}`);
  console.log(`main_model=${cfg.mainModel}`);
  console.log(`image_model=${cfg.imageModel}`);
  console.log(`api_key_present=${cfg.apiKey ? "yes" : "no"}`);
  if (!ossCfg) {
    console.log("oss_config=missing");
    return;
  }
  console.log("oss_config=present");
  console.log(`oss_endpoint=${ossCfg.endpoint}`);
  console.log(`oss_bucket=${ossCfg.bucket}`);
  console.log(`oss_prefix=${ossCfg.prefix}`);
  console.log(`oss_public_base_url=${ossCfg.publicBaseUrl || ""}`);
}

function sanitizeSegment(value) {
  value = String(value || "").trim().toLowerCase();
  let out = "";
  let lastDash = false;
  for (const ch of value) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9") || ch === "-" || ch === "_" || ch === ".") {
      out += ch;
      lastDash = false;
    } else if (!lastDash) {
      out += "-";
      lastDash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "").replace(/^\.+|\.+$/g, "");
}

function sanitizeFileName(fileName) {
  const base = path.basename(String(fileName || "").replace(/\\/g, "/"));
  const ext = path.extname(base).toLowerCase().replace(/[^.a-z0-9]/g, "");
  const stem = sanitizeSegment(path.basename(base, path.extname(base))) || "object";
  return `${stem}${ext}`;
}

function normalizeKeyPrefix(prefix) {
  return String(prefix || "")
    .trim()
    .split("/")
    .map(sanitizeSegment)
    .filter(Boolean)
    .join("/");
}

function generateOssKey(prefix, fileName) {
  const safeName = sanitizeFileName(fileName);
  const ext = path.extname(safeName);
  const stem = path.basename(safeName, ext) || "object";
  const now = new Date();
  const datePrefix = [
    now.getUTCFullYear(),
    String(now.getUTCMonth() + 1).padStart(2, "0"),
    String(now.getUTCDate()).padStart(2, "0"),
  ].join("/");
  const suffix = crypto.randomBytes(6).toString("hex");
  return [normalizeKeyPrefix(prefix), datePrefix, `${stem}-${suffix}${ext}`].filter(Boolean).join("/");
}

function hmac(key, value, encoding) {
  return crypto.createHmac("sha256", key).update(value, "utf8").digest(encoding);
}

function sha256(value, encoding = "hex") {
  return crypto.createHash("sha256").update(value).digest(encoding);
}

function encodePathPart(part) {
  return encodeURIComponent(part).replace(/[!'()*]/g, (ch) => `%${ch.charCodeAt(0).toString(16).toUpperCase()}`);
}

function ossObjectUrl(cfg, key) {
  const encodedKey = key.split("/").map(encodePathPart).join("/");
  if (cfg.publicBaseUrl) {
    return `${cfg.publicBaseUrl.replace(/\/+$/, "")}/${encodedKey}`;
  }
  const scheme = cfg.useSSL ? "https" : "http";
  return `${scheme}://${cfg.endpoint}/${encodePathPart(cfg.bucket)}/${encodedKey}`;
}

async function uploadToOss(filePath, cfg, contentType) {
  const body = fs.readFileSync(filePath);
  const key = generateOssKey(cfg.prefix, path.basename(filePath));
  const encodedPath = `/${encodePathPart(cfg.bucket)}/${key.split("/").map(encodePathPart).join("/")}`;
  const scheme = cfg.useSSL ? "https" : "http";
  const url = `${scheme}://${cfg.endpoint}${encodedPath}`;
  const now = new Date();
  const amzDate = now.toISOString().replace(/[:-]|\.\d{3}/g, "");
  const dateStamp = amzDate.slice(0, 8);
  const payloadHash = sha256(body);
  const headers = {
    "content-type": contentType,
    host: cfg.endpoint,
    "x-amz-content-sha256": payloadHash,
    "x-amz-date": amzDate,
  };
  const signedHeaders = Object.keys(headers).sort().join(";");
  const canonicalHeaders = Object.keys(headers)
    .sort()
    .map((name) => `${name}:${headers[name]}\n`)
    .join("");
  const canonicalRequest = ["PUT", encodedPath, "", canonicalHeaders, signedHeaders, payloadHash].join("\n");
  const scope = `${dateStamp}/${cfg.region}/s3/aws4_request`;
  const stringToSign = ["AWS4-HMAC-SHA256", amzDate, scope, sha256(canonicalRequest)].join("\n");
  const kDate = hmac(`AWS4${cfg.secretKey}`, dateStamp);
  const kRegion = hmac(kDate, cfg.region);
  const kService = hmac(kRegion, "s3");
  const kSigning = hmac(kService, "aws4_request");
  const signature = hmac(kSigning, stringToSign, "hex");
  const authorization = `AWS4-HMAC-SHA256 Credential=${cfg.accessKey}/${scope}, SignedHeaders=${signedHeaders}, Signature=${signature}`;

  const res = await fetch(url, {
    method: "PUT",
    headers: { ...headers, authorization },
    body,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`OSS upload failed for ${filePath}: HTTP ${res.status} ${res.statusText}: ${text}`);
  }
  return {
    bucket: cfg.bucket,
    key,
    uri: `oss://${cfg.bucket}/${key}`,
    url: ossObjectUrl(cfg, key),
  };
}

function detectMimeType(filePath) {
  const ext = path.extname(filePath).toLowerCase();
  if (ext === ".png") return "image/png";
  if (ext === ".jpg" || ext === ".jpeg") return "image/jpeg";
  if (ext === ".webp") return "image/webp";
  if (ext === ".gif") return "image/gif";
  throw new Error(`Unsupported image extension for ${filePath}. Use png, jpg, jpeg, webp, or gif.`);
}

function imageFileToDataUrl(filePath) {
  const abs = path.resolve(filePath);
  if (!fs.existsSync(abs)) {
    throw new Error(`Image file not found: ${filePath}`);
  }
  const mime = detectMimeType(abs);
  const b64 = fs.readFileSync(abs).toString("base64");
  return `data:${mime};base64,${b64}`;
}

function collectImageInputs(args) {
  const content = [];
  for (const filePath of args.image || []) {
    content.push({
      type: "input_image",
      image_url: imageFileToDataUrl(filePath),
    });
  }
  for (const url of args["image-url"] || []) {
    content.push({
      type: "input_image",
      image_url: url,
    });
  }
  return content;
}

function buildPayload(prompt, cfg, args) {
  const tool = {
    type: "image_generation",
    model: cfg.imageModel,
    size: args.size || "1024x1024",
    output_format: args.format || "png",
  };
  if (args.quality) tool.quality = args.quality;
  if (args.background) tool.background = args.background;
  if (args["partial-images"]) tool.partial_images = Number(args["partial-images"]);

  const content = [{ type: "input_text", text: prompt }, ...collectImageInputs(args)];

  return {
    model: cfg.mainModel,
    instructions: args.instructions || "Create the requested image.",
    input: [
      {
        type: "message",
        role: "user",
        content,
      },
    ],
    tools: [tool],
    tool_choice: { type: "image_generation" },
    stream: args.stream !== false,
    store: false,
  };
}

async function postResponses(cfg, payload) {
  const res = await fetch(`${cfg.baseUrl}/responses`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${cfg.apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    const body = await res.text();
    throw new Error(`HTTP ${res.status} ${res.statusText}: ${body}`);
  }
  return res;
}

function extractImageCalls(obj) {
  const calls = [];
  function visit(value) {
    if (!value || typeof value !== "object") return;
    if (value.type === "image_generation_call" && value.result) {
      calls.push(value);
    }
    if (Array.isArray(value)) {
      for (const item of value) visit(item);
      return;
    }
    for (const item of Object.values(value)) visit(item);
  }
  visit(obj);
  return calls;
}

async function handleJsonResponse(res, outputBase, format) {
  const json = await res.json();
  const calls = extractImageCalls(json);
  if (!calls.length) {
    throw new Error("No image_generation_call.result found in response.");
  }
  const files = [];
  calls.forEach((call, index) => {
    const suffix = calls.length > 1 ? `-${index + 1}` : "";
    const file = `${outputBase}${suffix}.${format}`;
    writeBase64(file, call.result);
    files.push({ file, revisedPrompt: call.revised_prompt || "" });
  });
  return files;
}

async function handleStreamResponse(res, outputBase, format) {
  const decoder = new TextDecoder();
  let buffer = "";
  let partialCount = 0;
  let sawPartialImage = false;
  const doneItems = [];
  const reader = res.body.getReader();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split(/\n\n/);
    buffer = events.pop() || "";
    for (const eventText of events) {
      const dataLines = eventText
        .split(/\r?\n/)
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trim());
      if (!dataLines.length) continue;
      const data = dataLines.join("\n");
      if (data === "[DONE]") continue;
      let event;
      try {
        event = JSON.parse(data);
      } catch {
        continue;
      }
      if (event.type === "error") {
        throw new Error(JSON.stringify(event.error || event));
      }
      if (event.type === "response.image_generation_call.partial_image" && event.partial_image_b64) {
        const idx = event.partial_image_index ?? partialCount;
        const file = `${outputBase}-partial-${idx}.${format}`;
        writeBase64(file, event.partial_image_b64);
        sawPartialImage = true;
        partialCount += 1;
        console.log(`partial_image=${file}`);
      }
      if (event.type === "response.output_item.done" && event.item) {
        doneItems.push(event.item);
      }
      if (event.type === "response.completed" && event.response) {
        doneItems.push(event.response);
      }
    }
  }

  const calls = extractImageCalls(doneItems);
  if (!calls.length && sawPartialImage) {
    throw new Error("Stream ended after partial image, but no final image_generation_call.result was returned.");
  }
  if (!calls.length) {
    throw new Error("No final image_generation_call.result found in stream.");
  }
  const files = [];
  calls.forEach((call, index) => {
    const suffix = calls.length > 1 ? `-${index + 1}` : "";
    const file = `${outputBase}${suffix}.${format}`;
    writeBase64(file, call.result);
    files.push({ file, revisedPrompt: call.revised_prompt || "" });
  });
  return files;
}

function isRetryableGenerationError(err) {
  return /partial image|final image_generation_call\.result|terminated/i.test(String(err && err.message));
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.version) {
    console.log(`script_version=${SCRIPT_VERSION}`);
    return;
  }
  const cfg = loadConfig(args);
  const ossCfg = loadOssConfig(args, cfg.env);
  if (args["check-config"]) {
    printConfigCheck(cfg, ossCfg);
    if (!ossCfg) {
      throw new Error("Missing OSS config.");
    }
    return;
  }
  const prompt = readPrompt(args);
  const format = args.format || "png";
  const contentType = `image/${format === "jpg" ? "jpeg" : format}`;
  const outDir = args["output-dir"] || "newapi-image-output";
  const stamp = timestamp();
  const slug = slugify(prompt);
  const promptDir = path.join(outDir, "prompt");
  const imageDir = path.join(outDir, "image");
  ensureDir(promptDir);
  ensureDir(imageDir);

  const promptFile = path.join(promptDir, `${slug}-${stamp}.md`);
  const imageFiles = (args.image || []).map((item) => path.resolve(item));
  const imageUrls = args["image-url"] || [];
  const archive = [
    prompt,
    "",
    imageFiles.length ? `Local reference images:\n${imageFiles.map((item) => `- ${item}`).join("\n")}` : "",
    imageUrls.length ? `Remote reference images:\n${imageUrls.map((item) => `- ${item}`).join("\n")}` : "",
  ].filter(Boolean).join("\n");
  fs.writeFileSync(promptFile, `${archive}\n`, "utf8");

  const outputBase = path.join(imageDir, `${slug}-${stamp}`);
  const maxAttempts = Math.max(1, Number(args.retries || args["max-attempts"] || 2));
  let files = null;
  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const payload = buildPayload(prompt, cfg, args);
    const attemptOutputBase = attempt === 1 ? outputBase : `${outputBase}-retry-${attempt}`;
    try {
      if (attempt > 1) console.log(`retry_attempt=${attempt}`);
      const res = await postResponses(cfg, payload);
      files = payload.stream
        ? await handleStreamResponse(res, attemptOutputBase, format)
        : await handleJsonResponse(res, attemptOutputBase, format);
      break;
    } catch (err) {
      if (attempt >= maxAttempts || !isRetryableGenerationError(err)) {
        throw err;
      }
      console.error(`retry_reason=${err.message}`);
    }
  }

  console.log(`prompt=${promptFile}`);
  if (!ossCfg) {
    console.log("oss_upload=skipped reason=missing_config");
  }
  for (const item of files) {
    console.log(`image=${item.file}`);
    if (ossCfg) {
      const uploaded = await uploadToOss(item.file, ossCfg, contentType);
      item.oss = uploaded;
      console.log(`oss_uri=${uploaded.uri}`);
      console.log(`oss_url=${uploaded.url}`);
    }
    if (item.revisedPrompt) console.log(`revised_prompt=${item.revisedPrompt}`);
  }
}

main().catch((err) => {
  console.error(err.message);
  process.exit(1);
});
