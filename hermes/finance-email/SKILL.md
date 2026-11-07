---
name: finance-email
description: "Use when processing bank/broker emails into the finance API. Fetches new iCloud mail read-only, parses transactions per parsing-rules.md, posts them."
version: 1.0.0
platforms: [linux]
metadata:
  hermes:
    tags: [finance, email, imap, cron]
---
# finance-email

Turns bank/broker notification emails (Jago, Stockbit, Krom, BCA, Bibit, SeaBank) into transactions in the finance API.
Runs from cron every 10 min. Parsing rules live in `parsing-rules.md` (same dir) — **read it before parsing**.

## Config (never write secrets into this skill)
From `~/.hermes/.env`: `IMAP_HOST`, `IMAP_USER`, `IMAP_PASSWORD`, `FINANCE_API_URL`, `FINANCE_API_TOKEN` (sent as `Authorization: Bearer`), `TELEGRAM_HOME_CHANNEL`, `FINANCE_FWD_FROM` (your own address that manually forwards bank emails), `FINANCE_OWNER_NAME` (your name as printed on bank transfers).
State in `~/.hermes/finance-email/`: `skipped.json` (Message-ID -> timestamp, 7-day TTL), `finance-email.log`.

## Pipeline
Scripts live in `~/.hermes/skills/finance-email/scripts/` (Python stdlib only).

1. **Fetch (script, no LLM)** — `scripts/fetch.py` (cron calls it through `~/.hermes/scripts/finance-email-fetch.py`):
   IMAP read-only (`select readonly=True`, `BODY.PEEK[]`) on INBOX + Junk, `SINCE` 3 days; keeps only allowlisted senders, including the original sender of manual `Fwd:` forwards from `FINANCE_FWD_FROM`; drops Message-IDs in `skipped.json`; asks `POST /api/source-refs/check`; keeps only `unknown`.
   Its last stdout line is JSON: `{"wakeAgent": false}` when nothing is new (cron then skips the LLM and sends nothing), otherwise `{"wakeAgent": true, "data": {"emails": [...], "rules": [...], "categories": [...]}}`.
2. **Parse (you, only when woken)** — for each email in `data.emails`, apply `parsing-rules.md`.
   - Merchant matches a rule in `data.rules` (case-insensitive *contains*) -> use that rule's `type` and `category`, `confidence` = 1.0. Check the real field names in the rules payload.
   - `category` must be a valid name for the `type` in `data.categories`.
   - Transaction fields: `occurred_at, amount, type, category, merchant, account, note, source_ref` (= Message-ID incl. `<>`), `confidence`, `needs_review` (true if confidence < 0.7 or anything flagged in the rules). No account numbers/balances anywhere.
   - Foreign-currency spend (e.g. Claude/Anthropic billed in USD): set `currency` (ISO, e.g. "USD") and `orig_amount` (e.g. 20.0).
     Set `amount` only when the email states the IDR actually debited; otherwise omit it and the API converts at that day's rate.
   - Investment order emails (Bibit/Stockbit: konfirmasi pembelian, pembelian berhasil/gagal, penjualan) -> NOT a transaction;
     put them in `orders` (see parsing-rules.md "Investment orders"). Reuse an existing name from `data.assets` when it is the same product;
     match a settlement email to `data.pending_orders` by order_ref.
   - Non-transaction emails (OTP, promo, statement, forwarding confirmation…) -> put the Message-ID in `skip`.
   - An email you cannot confidently classify either way: leave it out of both lists (it will be retried next run).
3. **Post (script)** — pipe JSON to `scripts/post.py`:
   ```
   echo '{"transactions":[{...}],"orders":[{...}],"skip":["<id>"]}' | python3 ~/.hermes/skills/finance-email/scripts/post.py
   ```
   It writes `skipped.json`, POSTs each transaction (201/409 = ok; 400 -> body logged; other errors logged and retried next run; no retries within a run), and prints `[SILENT]` (Kantong itself sends a Telegram message for every email transaction, review flag included).
4. **Final response** — output exactly what `post.py` printed on stdout (the summary or `[SILENT]`), nothing else. Cron delivers it to Telegram; `[SILENT]` suppresses delivery.

## Cron
`hermes cron` job `finance-email`, `every 10m`, `--script finance-email-fetch.py`, `--skill finance-email`, `--deliver telegram`.

## Manual run / debug
```
python3 ~/.hermes/skills/finance-email/scripts/fetch.py      # inspect gate output
tail ~/.hermes/finance-email/finance-email.log
```
