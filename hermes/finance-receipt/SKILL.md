---
name: finance-receipt
description: "Use when the user sends a photo/screenshot of a shopping receipt (struk) or payment proof and wants it recorded as a Kantong transaction."
version: 1.0.0
platforms: [linux]
metadata:
  hermes:
    tags: [finance, receipt, kantong]
---
# finance-receipt

Turns a receipt photo (Indomaret, Alfamart, restaurant, SPBU, QRIS proof…) into one Kantong transaction. No confirmation needed: post it, then tell the user what was recorded so they can correct it.
Config from the `.env` of this skill's Hermes profile (`~/.hermes/.env` in the default profile): `FINANCE_API_URL`, `FINANCE_API_TOKEN` (same as finance-email). Never print the token.

## Steps
1. Get valid categories and merchant rules:
   `python3 ${HERMES_SKILL_DIR}/scripts/receipt.py`
2. Read the receipt and build the transaction:
   - `amount`: the **grand total actually paid** (TOTAL / TOTAL BAYAR / after discounts, incl. tax). Integer rupiah, no separators. Not the cash handed over, not the change (KEMBALI).
   - `occurred_at`: date+time printed on the receipt, ISO with `+07:00` (e.g. `2026-11-08T14:05:00+07:00`). No date on it → now.
   - `merchant`: store name, short (e.g. `Indomaret`, `Alfamart`, `Starbucks`), without branch codes.
   - `type`: `out` (a refund receipt → `in`).
   - `category`: a rule whose pattern is contained in the merchant (case-insensitive) wins; otherwise the best-fitting `out` category from the list.
   - `account`: payment method printed on the receipt (`Cash`/`Tunai` → `Cash`, `Debit BCA` → `BCA`, `QRIS`/`GoPay`/`OVO`…). If the user said how they paid, use that. Unknown → `Cash`.
   - `note`: up to ~5 main items, comma-separated, ≤ 100 chars.
   - `split` (optional): when the items clearly belong to **2+ different categories** (e.g. snacks → Jajan, soap/tissue → Belanja, token listrik → Tagihan), list them as `[{"category","amount","note"}]` using the item line prices. The script merges same-category items and puts the discount/tax/rounding gap on the largest part; if the items are more than 15% off the total it skips the split and says so. All items in one category (or unreadable item lines) → no `split`.
   - `confidence`: 0.9 if total, date and merchant are clearly readable; lower otherwise (< 0.7 puts it in the Review queue).
   - Never include card numbers, member/phone numbers, or cashier names.
3. Post it:
   `echo '{"occurred_at":"...","amount":25000,"type":"out","category":"Jajan","merchant":"Indomaret","account":"Cash","note":"Aqua, Roti","confidence":0.9}' | python3 ${HERMES_SKILL_DIR}/scripts/receipt.py post`
   With a split: `{..., "amount":150000, "category":"Jajan", "split":[{"category":"Jajan","amount":35000,"note":"Chitato, kopi"},{"category":"Belanja","amount":85000,"note":"Rinso, tisu"},{"category":"Tagihan","amount":30000,"note":"token listrik"}]}` (`category` = the largest part's).
   It prints one line (`✓ Keluar Rp25.000 · Jajan · Indomaret · Cash`, plus `· split: Jajan Rp35.000, Belanja Rp85.000, …` when split, `sudah tercatat` for the same receipt sent twice, or `✗ …` with the API error). Reply with that line; if a field was a guess, say which one and that it can be fixed in the app.
4. Several receipts in one message → post each separately.

## Corrections, shared bills, transfers
- Bank-email transactions often exist already (source `email`). Before posting or adjusting, `GET /api/transactions` and check for the same amount/time to avoid double entries. Found one and the receipt has items in several categories → split that transaction instead of posting: `echo '{"amount":<its amount>,"split":[...]}' | python3 ${HERMES_SKILL_DIR}/scripts/receipt.py split <id>`.
- Split rules (API `POST /api/transactions/{id}/split` with `{"parts":[{"category","amount","note"}]}`): parts must add up exactly to the total; `in`/`out` IDR only (no transfers); calling it again on any part replaces the split; one part merges it back.
- Split bill (patungan): the full payment stays as the user's `out`; the friend's share is a separate `in` (category `Refund`, merchant = friend, account = where it was received, note names the items).
- Swapping balance for cash with a friend: the transfer is `transfer` from the paying account to `Cash` (cash received), and the friend's share is another `in` to `Cash`.
- Transfers to a friend are NOT own-account transfers: clear `to_account` and `merchant`, set `needs_review:true`. Never take `to_account` from a bank's generic "X to Y" label.
- Transactions: `PATCH /api/transactions/{id}` works. Rules: `PATCH /api/rules/{id}` edits `pattern`/`type`/`category` in place on current Kantong builds. If it answers 405 (older deploy), DELETE the rule then POST it again (the id changes). The server also flags email transfers without `to_account` as Review on current builds; still clear `to_account`/`merchant` yourself when fixing one.
- Kantong code changes (new endpoints, UI) only reach the running app after a rebuild/redeploy: merging to `main` is not enough, so tell the user when a fix is merged but not live.
- Categories (when the user asks in chat to add, rename or delete one): `GET /api/categories` gives `id`, `name`, `type`, `custom`. Add: `POST /api/categories` `{"name","type"}` (`in`/`out`). Rename: `PATCH /api/categories/{id}` `{"name"}`; transactions, rules, budgets and recurring items follow. Delete: `DELETE /api/categories/{id}`; its transactions and recurring items move to `Lainnya`, its rules and budgets are dropped, so confirm with the user first. Only `custom: true` categories can be renamed or deleted (built-ins answer 403). Read category names from the API each time, never from memory: an old name is rejected after a rename or delete.
- Category hints: pulsa, paket data, token listrik, internet, air → `Tagihan`. After the user fixes a category in-app, check `/api/rules` matches it (a rule pointing elsewhere re-categorises future ones).
