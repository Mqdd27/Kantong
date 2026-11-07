#!/usr/bin/env python3
"""finance-email step 1-3: read-only IMAP fetch -> filter -> dedupe -> API check.

Prints ONE line of JSON as the last stdout line (cron wake-gate convention):
  {"wakeAgent": false}                      -> nothing to do, no LLM, no message
  {"wakeAgent": true, "data": {...}}        -> unknown emails + rules for the LLM to parse
"""
import sys, os, re, json, imaplib, email, html
from email import policy
from email.utils import parseaddr
from datetime import datetime, timedelta

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import load_env, log, load_skipped, api, WIB

ALLOW = {"noreply@jago.com", "no-reply@stockbit.com", "noreply@krom.id", "bca@bca.co.id",
         "pasporbca@klikbca.com", "no-reply@bibit.id", "alerts@seabank.co.id"}
FOLDERS = ("INBOX", "Junk")
DAYS = 3
MAX_BODY = 6000
MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
FWD_RE = re.compile(r"-{5,}\s*Forwarded message\s*-{5,}\s*(.*)", re.I | re.S)
FROM_RE = re.compile(r"^\s*From:\s*(.+)$", re.I | re.M)


def done(wake, data=None):
    out = {"wakeAgent": wake}
    if data is not None:
        out["data"] = data
    print(json.dumps(out, ensure_ascii=False))
    sys.exit(0)


def bodies(msg):
    plain = html_ = ""
    for part in msg.walk():
        if part.is_multipart() or part.get_content_disposition() == "attachment":
            continue
        ct = part.get_content_type()
        try:
            txt = part.get_content()
        except Exception:
            continue
        if ct == "text/plain" and not plain:
            plain = txt
        elif ct == "text/html" and not html_:
            html_ = txt
    if not plain and html_:
        t = re.sub(r"(?is)<(script|style).*?</\1>", "", html_)
        t = re.sub(r"(?i)<br\s*/?>|</(p|div|tr|li|h\d)>", "\n", t)
        t = re.sub(r"(?s)<[^>]+>", " ", t)
        plain = html.unescape(re.sub(r"[ \t\xa0]+", " ", t))
        plain = re.sub(r"\n\s*\n+", "\n\n", plain)
    # drop long tracking URLs (token waste, no signal)
    plain = re.sub(r"<https?://[^>\s]{40,}>", "", plain)
    return plain


def effective_sender(msg, text, fwd_from=""):
    """Return (allowlisted_addr or None, via)."""
    frm = parseaddr(str(msg.get("From", "")))[1].lower()
    if frm in ALLOW:
        return frm, "direct"
    subj = str(msg.get("Subject", "")).strip()
    if fwd_from and frm == fwd_from and subj.lower().startswith("fwd:"):
        m = FWD_RE.search(text)
        if m:
            fm = FROM_RE.search(m.group(1))
            if fm:
                orig = parseaddr(html.unescape(fm.group(1)).strip())[1].lower()
                if orig in ALLOW:
                    return orig, "manual_forward"
    return None, None


def main():
    env = load_env()
    for k in ("IMAP_HOST", "IMAP_USER", "IMAP_PASSWORD", "FINANCE_API_URL", "FINANCE_API_TOKEN"):
        if not env.get(k):
            log(f"FETCH config missing: {k}")
            done(False)

    d = datetime.now(WIB) - timedelta(days=DAYS)
    since = f"{d.day:02d}-{MONTHS[d.month - 1]}-{d.year}"
    found = {}
    try:
        m = imaplib.IMAP4_SSL(env["IMAP_HOST"], timeout=30)
        m.login(env["IMAP_USER"], env["IMAP_PASSWORD"])
        for folder in FOLDERS:
            typ, _ = m.select(f'"{folder}"', readonly=True)
            if typ != "OK":
                log(f"FETCH cannot select {folder}")
                continue
            typ, data = m.search(None, "SINCE", since)
            if typ != "OK" or not data or not data[0]:
                continue
            for num in data[0].split():
                typ, md = m.fetch(num, "(BODY.PEEK[])")
                if typ != "OK" or not md or not isinstance(md[0], tuple):
                    continue
                msg = email.message_from_bytes(md[0][1], policy=policy.default)
                mid = str(msg.get("Message-ID", "")).strip()
                if not mid:
                    continue
                text = bodies(msg)
                sender, via = effective_sender(msg, text, env.get("FINANCE_FWD_FROM", "").lower())
                if not sender or mid in found:
                    continue
                found[mid] = {
                    "message_id": mid, "folder": folder, "sender": sender, "via": via,
                    "subject": str(msg.get("Subject", "")), "header_date": str(msg.get("Date", "")),
                    "body": text[:MAX_BODY],
                }
        try:
            m.logout()
        except Exception:
            pass
    except Exception as e:
        log(f"FETCH imap error: {type(e).__name__}: {e}")
        done(False)

    skipped = load_skipped()
    cand = [v for k, v in found.items() if k not in skipped]
    if not cand:
        done(False)

    try:
        st, res = api(env, "POST", "/api/source-refs/check", {"refs": [c["message_id"] for c in cand]})
        if st != 200 or not isinstance(res, dict):
            log(f"FETCH check HTTP {st}: {str(res)[:300]}")
            done(False)
        unknown = set(res.get("unknown") or [])
        todo = [c for c in cand if c["message_id"] in unknown]
        if not todo:
            done(False)
        st, rules = api(env, "GET", "/api/rules")
        if st != 200:
            log(f"FETCH rules HTTP {st}: {str(rules)[:300]}")
            rules = []
        st, cats = api(env, "GET", "/api/categories")
        if st != 200:
            cats = []
    except Exception as e:
        log(f"FETCH api error: {type(e).__name__}: {e}")
        done(False)

    log(f"FETCH {len(todo)} unknown email(s) -> waking agent")
    try:
        st, pf = api(env, "GET", "/api/assets")
        assets = [{"name": a["name"], "kind": a["kind"], "currency": a["currency"]} for a in pf["assets"]] if st == 200 else []
        pending = [{"asset": o["asset"], "order_ref": o["order_ref"], "side": o["side"], "amount": o["amount"]}
                   for o in pf["pending_orders"]] if st == 200 else []
    except Exception as e:
        log(f"FETCH assets error: {e}")
        assets, pending = [], []
    done(True, {"emails": todo, "rules": rules, "categories": cats, "assets": assets, "pending_orders": pending,
          "owner_name": env.get("FINANCE_OWNER_NAME", "")})


if __name__ == "__main__":
    main()
