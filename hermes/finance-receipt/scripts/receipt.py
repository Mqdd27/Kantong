#!/usr/bin/env python3
"""finance-receipt: print categories + rules, `post` one transaction (JSON on stdin, optional "split"),
or `split <id>` an existing transaction ({"amount": total, "split": [...]} on stdin)."""
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


def fit_parts(items, total):
    """Merge items per category and make them add up to total: the gap (discount, tax, rounding)
    goes to the largest part. None when the gap is too big to be rounding (> 15%) or < 2 categories."""
    by = {}
    for it in items:
        c, a = it["category"], int(it["amount"])
        p = by.setdefault(c, {"category": c, "amount": 0, "notes": []})
        p["amount"] += a
        if it.get("note"):
            p["notes"].append(it["note"])
    parts = [{"category": p["category"], "amount": p["amount"], "note": ", ".join(p["notes"])[:100]} for p in by.values()]
    gap = total - sum(p["amount"] for p in parts)
    if len(parts) < 2 or abs(gap) > total * 0.15:
        return None
    big = max(parts, key=lambda p: p["amount"])
    big["amount"] += gap
    return parts if big["amount"] > 0 else None


def split(e, tx_id, items, total):
    parts = fit_parts(items, total)
    if not parts:
        return "split dilewati (item tidak cocok dengan total atau cuma 1 kategori)"
    st, body = api(e, "POST", f"/api/transactions/{tx_id}/split", {"parts": parts})
    if st != 200:
        return f"split gagal: {st} {body}"
    return "split: " + ", ".join(f"{p['category']} {rupiah(p['amount'])}" for p in parts)


def main():
    e = env()
    if sys.argv[1:2] == ["selftest"]:
        assert fit_parts([{"category": "Jajan", "amount": 20000}, {"category": "Jajan", "amount": 15000},
                          {"category": "Belanja", "amount": 90000}], 120000) == [
            {"category": "Jajan", "amount": 35000, "note": ""}, {"category": "Belanja", "amount": 85000, "note": ""}]
        assert fit_parts([{"category": "Jajan", "amount": 10000}, {"category": "Belanja", "amount": 10000}], 50000) is None
        assert fit_parts([{"category": "Jajan", "amount": 10000}], 10000) is None
        print("ok")
        return
    if sys.argv[1:2] == ["split"] and len(sys.argv) == 3:
        b = json.load(sys.stdin)
        print(split(e, int(sys.argv[2]), b["split"], int(b["amount"])))
        return
    if sys.argv[1:] != ["post"]:
        _, cats = api(e, "GET", "/api/categories")
        _, rules = api(e, "GET", "/api/rules")
        print(json.dumps({"categories": cats, "rules": rules}, ensure_ascii=False))
        return
    t = json.load(sys.stdin)
    items = t.pop("split", None)
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
        line = " · ".join(p for p in parts if p) + (" (perlu review)" if t["needs_review"] else "")
        if items:
            line += " · " + split(e, body["id"], items, body["amount"])
        print(line)
    else:
        print(f"✗ {st} {body}")


if __name__ == "__main__":
    main()
