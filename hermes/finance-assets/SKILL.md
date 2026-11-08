---
name: finance-assets
description: "Use when the user sends a screenshot (or text) of an investment/bank portfolio — Bibit, Stockbit, bank app, etc. — and wants Kantong assets created or updated from it."
version: 1.0.0
platforms: [linux]
metadata:
  hermes:
    tags: [finance, assets, kantong]
---
# finance-assets

Reads a portfolio screenshot the user sends and creates/updates assets in Kantong.
Config comes from `~/.hermes/.env`: `FINANCE_API_URL`, `FINANCE_API_TOKEN` (same as finance-email). Never print the token.

## Steps
1. List current assets (compact JSON, one per line):
   `python3 ~/.hermes/skills/finance-assets/scripts/assets.py`
2. Read the screenshot and build one object per product. Match an existing asset by name (same series/product, case-insensitive) and include its `id` to update it; no match → no `id` (creates one).
3. Show the user what you will write (name, values, new vs update) and wait for "ok". Then apply:
   `echo '{"assets":[{...}]}' | python3 ~/.hermes/skills/finance-assets/scripts/assets.py apply`
   It PATCHes (only the fields you send) or POSTs, and prints one result line per asset. Reply with those lines.

## Fields
`name, kind, currency (default IDR), balance, invested, note` for every asset. Numbers are plain (no "Rp", no thousand separators).
`kind`: `bank, ewallet, cash, forex, stock, bond, sbn, mutual_fund, gold, crypto, other` (check the list output for what the user already uses).

| Product | Fields |
|---|---|
| Government bond FR/PBS/ORI/SR/ST/SBR | `kind:"sbn"`; `balance` = modal + total keuntungan (current value); `invested` = Total Modal Investasi; `face` = units × 1.000.000 (nominal); `return_pct` = **Kupon / Tahun** (NOT yield); `payout` = 6 for FR/PBS, 1 for ORI/SR/ST/SBR; `maturity` = Jatuh Tempo as YYYY-MM-DD |
| Corporate bond | `kind:"bond"`, same fields; `payout` from the coupon schedule, 0 if no coupon |
| Mutual fund | `kind:"mutual_fund"`; `balance` = nilai sekarang; `invested` = modal; `return_pct` only if the user gives an expected return |
| Stock (total) | `kind:"stock"`; `balance`, `invested` |
| Bank / e-wallet | `balance`; for bank `return_pct` = bunga % per tahun if shown |

Skip anything you cannot read clearly and say which field was unclear. Never store account numbers.
