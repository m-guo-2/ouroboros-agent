#!/usr/bin/env python3
"""Mirror a remote SQLite database to a local .db file via the
http://<host>/db_view/api/query JSON RPC.

Why: production runs the legacy SQLite catalog behind an HTTP query
proxy; we don't have file-level access to it. The MySQL importer
(`cmd/migrate-sqlite-to-mysql`) reads a *local* .db, so this script
exists to materialise that local copy. Once it finishes, run the Go
importer against the produced file.

Usage:
    python3 mirror_via_api.py \\
        --endpoint http://115.190.14.209/db_view/api/query \\
        --output ./agent.sqlite
"""
from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
import time
import urllib.request


def post(endpoint: str, sql: str, timeout: int = 60):
    req = urllib.request.Request(
        endpoint,
        data=json.dumps({"sql": sql}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read()
    payload = json.loads(body)
    if not payload.get("success"):
        raise RuntimeError(f"API error for SQL={sql!r}: {payload!r}")
    return payload.get("data", []) or []


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--endpoint", required=True)
    ap.add_argument("--output", required=True, help="path to local .db (will be overwritten)")
    ap.add_argument("--page", type=int, default=2000, help="rows per SELECT page")
    args = ap.parse_args()

    if os.path.exists(args.output):
        os.remove(args.output)

    schema = post(
        args.endpoint,
        "SELECT type, name, tbl_name, sql "
        "FROM sqlite_master "
        "WHERE name NOT LIKE 'sqlite_%' "
        "ORDER BY CASE type WHEN 'table' THEN 0 WHEN 'index' THEN 1 ELSE 2 END, name",
    )

    tables: list[str] = []
    conn = sqlite3.connect(args.output)
    try:
        cur = conn.cursor()
        for row in schema:
            sql = row.get("sql")
            if not sql:
                continue
            if row["type"] == "table":
                tables.append(row["name"])
            try:
                cur.execute(sql)
            except sqlite3.Error as e:
                print(f"  ! skipping {row['type']} {row['name']}: {e}", file=sys.stderr)
        conn.commit()

        for tbl in tables:
            print(f"[{tbl}] streaming...", flush=True)
            cols_meta = post(args.endpoint, f"PRAGMA table_info({tbl})")
            col_names = [c["name"] for c in cols_meta]
            placeholders = ",".join(["?"] * len(col_names))
            quoted_cols = ",".join(f'"{c}"' for c in col_names)
            insert = f'INSERT INTO "{tbl}" ({quoted_cols}) VALUES ({placeholders})'

            offset = 0
            total = 0
            while True:
                started = time.time()
                page = post(
                    args.endpoint,
                    f'SELECT {quoted_cols} FROM "{tbl}" LIMIT {args.page} OFFSET {offset}',
                    timeout=120,
                )
                if not page:
                    break
                cur.executemany(
                    insert,
                    [tuple(r.get(c) for c in col_names) for r in page],
                )
                conn.commit()
                total += len(page)
                offset += len(page)
                print(
                    f"  +{len(page):>5}  total={total:<7}  ({time.time() - started:.2f}s)",
                    flush=True,
                )
                if len(page) < args.page:
                    break
            print(f"  done {tbl} = {total} rows", flush=True)
    finally:
        conn.close()

    print("OK", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
