#!/usr/bin/env python3
"""Amap REST API client. Shared helper for all scripts."""

import json
import os
import sys
import urllib.request
import urllib.error
import urllib.parse

BASE_URL = "https://restapi.amap.com"
DEFAULT_AMAP_API_KEY = "a755be9bdb9f93b2b4c0844c2fb3f0af"


def get_key():
    key = os.environ.get("AMAP_API_KEY", DEFAULT_AMAP_API_KEY)
    if not key:
        print(json.dumps({"error": "AMAP_API_KEY 未配置"}))
        sys.exit(1)
    return key


def call_amap(path, params):
    """GET request to amap API. Returns parsed JSON."""
    params["key"] = get_key()
    qs = urllib.parse.urlencode(params)
    url = f"{BASE_URL}{path}?{qs}"

    req = urllib.request.Request(url)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        return {"error": f"HTTP {e.code}"}
    except urllib.error.URLError as e:
        return {"error": f"连接失败: {e.reason}"}

    if data.get("status") == "0":
        return {"error": data.get("info", "请求失败"), "infocode": data.get("infocode")}
    return data


def output(data):
    print(json.dumps(data, ensure_ascii=False, indent=2))


def parse_args(argv, required=None):
    required = required or []
    args = {}
    i = 0
    while i < len(argv):
        if argv[i].startswith("--") and i + 1 < len(argv):
            args[argv[i][2:]] = argv[i + 1]
            i += 2
        else:
            i += 1
    missing = [k for k in required if k not in args]
    if missing:
        output({"error": f"缺少必填参数: {', '.join('--' + k for k in missing)}"})
        sys.exit(1)
    return args
