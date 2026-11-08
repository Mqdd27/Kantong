#!/usr/bin/env python3
"""finance-receipt: print categories + rules, or `post` one transaction (JSON on stdin) to Kantong."""
import hashlib, json, os, sys, urllib.error, urllib.request
from pathlib import Path


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


def rupiah(n):
    return "Rp" + f"{n:,}".replace(",", ".")


def main():
    e = env()
    if sys.argv[1:] != ["post"]:
        _, cats = api(e, "GET", "/api/categories")
        _, rules = api(e, "GET", "/api/rules")
        print(json.dumps({"categories": cats, "rules": rules}, ensure_ascii=False))
        return
    t = json.load(sys.stdin)
    t["source"] = "manual"
    t["amount"] = int(t["amount"])
    # same receipt sent twice -> same ref -> API answers 409 instead of a duplicate
    key = f"{t.get('merchant', '').lower()}|{t.get('occurred_at', '')}|{t['amount']}"
    t["source_ref"] = "receipt:" + hashlib.sha1(key.encode()).hexdigest()[:16]
    t["needs_review"] = bool(t.get("needs_review")) or float(t.get("confidence", 0.9)) < 0.7
    st, body = api(e, "POST", "/api/transactions", t)
    if st == 409:
        print(f"sudah tercatat: {t.get('merchant')} {rupiah(t['amount'])}")
    elif st == 201:
        verb = {"in": "Masuk", "out": "Keluar", "transfer": "Transfer"}.get(t["type"], t["type"])
        parts = [f"✓ {verb} {rupiah(t['amount'])}", t["category"], t.get("merchant"), t.get("account")]
        print(" · ".join(p for p in parts if p) + (" (perlu review)" if t["needs_review"] else ""))
    else:
        print(f"✗ {st} {body}")


if __name__ == "__main__":
    main()
