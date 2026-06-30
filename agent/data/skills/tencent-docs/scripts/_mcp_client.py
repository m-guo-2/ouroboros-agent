#!/usr/bin/env python3
"""Tencent Docs MCP client. Shared helper for all scripts."""

import json
import os
import sys
import time
import urllib.request
import urllib.error


MCP_BASE_URL = "https://docs.qq.com/openapi/mcp"
DEFAULT_TENCENT_DOCS_TOKEN = "3e6f433e4aad4525b6cc62cacd8b7eea"


def get_config():
    token = os.environ.get("TENCENT_DOCS_TOKEN", DEFAULT_TENCENT_DOCS_TOKEN)
    if not token:
        print(json.dumps({"error": "TENCENT_DOCS_TOKEN 未配置。请访问 https://docs.qq.com/open/auth/mcp.html 获取"}))
        sys.exit(1)
    return token, MCP_BASE_URL


def call_mcp(tool_name, arguments):
    """Call a tencent docs MCP tool via JSON-RPC (Streamable HTTP) and return the parsed result."""
    token, base_url = get_config()
    payload = json.dumps({
        "jsonrpc": "2.0",
        "method": "tools/call",
        "params": {"name": tool_name, "arguments": arguments},
        "id": 1,
    }).encode("utf-8")

    req = urllib.request.Request(base_url, data=payload, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", f"Bearer {token}")

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        return {"error": f"HTTP {e.code}: {body}"}
    except urllib.error.URLError as e:
        return {"error": f"连接失败: {e.reason}"}

    if "error" in data:
        err = data["error"]
        msg = err.get("message", str(err)) if isinstance(err, dict) else str(err)
        return {"error": msg}

    result = data.get("result", {})
    if isinstance(result, dict):
        content = result.get("content")
        if isinstance(content, list):
            texts = [c.get("text", "") for c in content
                     if isinstance(c, dict) and c.get("type") == "text"]
            if texts:
                try:
                    return json.loads(texts[0])
                except json.JSONDecodeError:
                    return {"text": texts[0]}
    return result


def poll_progress(check_fn, interval=3, timeout=300):
    """Poll an async operation until completion or timeout."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        result = check_fn()
        if isinstance(result, dict) and result.get("error"):
            return result
        status = result.get("status") if isinstance(result, dict) else None
        if status == 2 or status == "completed":
            return result
        time.sleep(interval)
    return {"error": "操作超时"}


def output(data):
    """Print JSON result to stdout."""
    print(json.dumps(data, ensure_ascii=False, indent=2))


def parse_args(argv, required=None, optional=None):
    """Parse --key value pairs from argv. Returns dict."""
    required = required or []
    optional = optional or []
    args = {}
    i = 0
    while i < len(argv):
        if argv[i].startswith("--") and i + 1 < len(argv):
            key = argv[i][2:]
            args[key] = argv[i + 1]
            i += 2
        else:
            i += 1
    missing = [k for k in required if k not in args]
    if missing:
        output({"error": f"缺少必填参数: {', '.join('--' + k for k in missing)}"})
        sys.exit(1)
    return args
