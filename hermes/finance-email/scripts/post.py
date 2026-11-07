#!/usr/bin/env python3
"""finance-email step 5-7: read agent JSON on stdin, record skips, POST transactions.

stdin: {"transactions": [{occurred_at, amount, type, category, merchant, account, note,
                          source_ref, confidence, needs_review?, currency?, orig_amount?}, ...],
        "orders": [{asset, kind, currency?, side, amount, status, order_ref, source_ref, occurred_at, note?}, ...],
        "skip": ["<message-id>", ...]}
stdout: a ready-to-send Telegram text if any posted/known tx needs review, else "[SILENT]".
"""
import sys, os, json
from datetime import datetime

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import load_env, log, load_skipped, save_skipped, api, WIB

REQ = ("occurred_at", "type", "category", "source_ref")
ORDER_REQ = ("asset", "side", "amount", "status", "order_ref", "source_ref")


def main():
    env = load_env()
    try:
        payload = json.load(sys.stdin)
    except ValueError as e:
        log(f"POST bad stdin JSON: {e}")
        print("[SILENT]")
        return

    now = datetime.now(WIB).isoformat(timespec="seconds")
    skip = [s for s in payload.get("skip", []) if isinstance(s, str) and s]
    if skip:
        sk = load_skipped()
        for s in skip:
            sk[s] = now
        save_skipped(sk)
        log(f"POST skipped {len(skip)} non-transaction email(s)")

    review, results = [], []
    for t in payload.get("transactions", []):
        t = dict(t)
        t["source"] = "email"
        try:
            t["currency"] = (t.get("currency") or "IDR").upper()
            if t["currency"] == "IDR":
                t["amount"] = int(t["amount"])
                t.pop("orig_amount", None)
            else:  # foreign spend: orig_amount required, IDR amount optional (API converts)
                t["orig_amount"] = float(t["orig_amount"])
                t["amount"] = int(t.get("amount") or 0)
            t["confidence"] = float(t.get("confidence", 0.8))
        except (TypeError, ValueError, KeyError):
            log(f"POST invalid amount/confidence ref={t.get('source_ref')}")
            continue
        t["needs_review"] = bool(t.get("needs_review")) or t["confidence"] < 0.7
        miss = [k for k in REQ if not t.get(k)]
        if miss:
            log(f"POST skip invalid tx ref={t.get('source_ref')} missing={miss}")
            continue
        try:
            st, body = api(env, "POST", "/api/transactions", t)
        except Exception as e:
            log(f"POST error ref={t['source_ref']}: {type(e).__name__}: {e}")
            results.append((t["source_ref"], "error"))
            continue
        if st in (201, 409):
            results.append((t["source_ref"], st))
            if st == 201 and t["needs_review"]:
                review.append(t)
        elif st == 400:
            log(f"POST 400 ref={t['source_ref']} body={json.dumps(body, ensure_ascii=False)[:500]}")
            results.append((t["source_ref"], 400))
        else:
            log(f"POST HTTP {st} ref={t['source_ref']} body={str(body)[:300]}")
            results.append((t["source_ref"], st))

    done_orders = []
    for o in payload.get("orders", []):
        o = dict(o)
        miss = [k for k in ORDER_REQ if not o.get(k)]
        if miss:
            log(f"ORDER skip invalid ref={o.get('order_ref')} missing={miss}")
            continue
        try:
            st, body = (404, None)
            if o["status"] != "pending":  # settle an existing pending order first
                st, body = api(env, "PATCH", f"/api/orders/{o['order_ref']}", {"status": o["status"]})
            if st == 404:
                st, body = api(env, "POST", "/api/orders", o)
        except Exception as e:
            log(f"ORDER error ref={o['order_ref']}: {type(e).__name__}: {e}")
            continue
        log(f"ORDER {o['order_ref']} {o['status']} -> HTTP {st} {str(body)[:200] if st >= 400 else ''}")
        results.append((o["source_ref"], st))
        if st in (200, 201, 409):
            done_orders.append(o["source_ref"])  # not a transaction: remember it so fetch won't wake again
    if done_orders:
        sk = load_skipped()
        for r in done_orders:
            sk[r] = now
        save_skipped(sk)

    log("POST results: " + ", ".join(f"{r}={s}" for r, s in results))
    print(json.dumps({"results": results}, ensure_ascii=False), file=sys.stderr)
    if True:  # Kantong sends a Telegram message per email transaction itself (tahap 4)
        print("[SILENT]")
        return
    lines = ["⚠️ Transaksi perlu dicek:"]
    for t in review:
        try:
            d = datetime.fromisoformat(t["occurred_at"]).strftime("%d %b %Y %H:%M")
        except ValueError:
            d = t["occurred_at"]
        amt = f"Rp {t['amount']:,}".replace(",", ".")
        lines.append(f"• {d} | {amt} | {t.get('merchant') or '-'} | tebakan: {t['category']} ({t['type']})")
    print("\n".join(lines))


if __name__ == "__main__":
    main()
