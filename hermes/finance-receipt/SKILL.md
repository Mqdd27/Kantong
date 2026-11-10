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
Config from `~/.hermes/.env`: `FINANCE_API_URL`, `FINANCE_API_TOKEN` (same as finance-email). Never print the token.

## Steps
1. Get valid categories and merchant rules:
   `python3 ~/.hermes/skills/finance-receipt/scripts/receipt.py`
2. Read the receipt and build the transaction:
   - `amount`: the **grand total actually paid** (TOTAL / TOTAL BAYAR / after discounts, incl. tax). Integer rupiah, no separators. Not the cash handed over, not the change (KEMBALI).
   - `occurred_at`: date+time printed on the receipt, ISO with `+07:00` (e.g. `2026-11-08T14:05:00+07:00`). No date on it → now.
   - `merchant`: store name, short (e.g. `Indomaret`, `Alfamart`, `Starbucks`), without branch codes.
   - `type`: `out` (a refund receipt → `in`).
   - `category`: a rule whose pattern is contained in the merchant (case-insensitive) wins; otherwise the best-fitting `out` category from the list.
   - `account`: payment method printed on the receipt (`Cash`/`Tunai` → `Cash`, `Debit BCA` → `BCA`, `QRIS`/`GoPay`/`OVO`…). If the user said how they paid, use that. Unknown → `Cash`.
   - `note`: up to ~5 main items, comma-separated, ≤ 100 chars.
   - `confidence`: 0.9 if total, date and merchant are clearly readable; lower otherwise (< 0.7 puts it in the Review queue).
   - Never include card numbers, member/phone numbers, or cashier names.
3. Post it:
   `echo '{"occurred_at":"...","amount":25000,"type":"out","category":"Jajan","merchant":"Indomaret","account":"Cash","note":"Aqua, Roti","confidence":0.9}' | python3 ~/.hermes/skills/finance-receipt/scripts/receipt.py post`
   It prints one line (`✓ Keluar Rp25.000 · Jajan · Indomaret · Cash`, `sudah tercatat` for the same receipt sent twice, or `✗ …` with the API error). Reply with that line; if a field was a guess, say which one and that it can be fixed in the app.
4. Several receipts in one message → post each separately.

## Corrections, shared bills, transfers
- Bank-email transactions often exist already (source `email`). Before posting or adjusting, `GET /api/transactions` and check for the same amount/time to avoid double entries.
- Split bill (patungan): the full payment stays as the user's `out`; the friend's share is a separate `in` (category `Refund`, merchant = friend, account = where it was received, note names the items).
- Swapping balance for cash with a friend: the transfer is `transfer` from the paying account to `Cash` (cash received), and the friend's share is another `in` to `Cash`.
- Transfers to a friend are NOT own-account transfers: clear `to_account` and `merchant`, set `needs_review:true`. Never take `to_account` from a bank's generic "X to Y" label.
- Transactions: `PATCH /api/transactions/{id}` works. Rules (`/api/rules`) have only GET/POST/DELETE: to change a rule, DELETE it then POST it again.
- Category hints: pulsa, paket data, token listrik, internet, air → `Tagihan`. After the user fixes a category in-app, check `/api/rules` matches it (a rule pointing elsewhere re-categorises future ones).
