#!/usr/bin/env python3
"""finance-assets: list Kantong assets, or apply {"assets":[...]} from stdin (id -> PATCH, else POST)."""
import json, os, sys, urllib.error, urllib.request
from pathlib import Path

KEEP = ("id", "name", "kind", "currency", "balance", "invested", "face", "return_pct", "payout", "maturity", "note")


def env():
    e = dict(os.environ)
    p = Path.home() / ".hermes" / ".env"
    if p.exists():
        for line in p.read_text().splitlines():
            if "=" in line and not line.lstrip().startswith("#"):
                k, v = line.split("=", 1)
                e.setdefault(k.strip(), v.strip().strip("\"'"))
    return e


def api(e, method, path, body=None):
    req = urllib.request.Request(e["FINANCE_API_URL"].rstrip("/") + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers={"Authorization": "Bearer " + e["FINANCE_API_TOKEN"], "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode("utf-8", "replace")


def main():
    e = env()
    if sys.argv[1:] != ["apply"]:
        st, body = api(e, "GET", "/api/assets")
        if st != 200:
            sys.exit(f"list failed: {st} {body}")
        for a in body["assets"]:
            print(json.dumps({k: a.get(k) for k in KEEP}, ensure_ascii=False))
        return
    for a in json.load(sys.stdin).get("assets", []):
        a = dict(a)
        aid = a.pop("id", None)
        st, body = api(e, "PATCH", f"/api/assets/{aid}", a) if aid else api(e, "POST", "/api/assets", a)
        ok = st in (200, 201)
        print(f"{'✓' if ok else '✗'} {'update' if aid else 'baru'} {a.get('name', aid)}" + ("" if ok else f": {st} {body}"))


if __name__ == "__main__":
    main()
