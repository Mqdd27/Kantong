# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

One person: the owner, managing their own money. Single password, no other accounts. They open Kantong on the phone (installed PWA) and on desktop about equally: the phone for a quick check or a manual entry on the go, the desktop for budgeting, analysis, and asset upkeep.

## Product Purpose

A self-hosted personal money manager. Bank, e-wallet, and broker notification emails are parsed by an agent (Hermes) and posted to the API, so transactions appear in real time without manual entry. The owner reviews uncertain ones, keeps a monthly budget, tracks recurring bills, and watches net worth across accounts and investments. Success: knowing where the money went this month and what is left, with almost no typing.

## Positioning

Transactions arrive by themselves from bank emails (and receipt photos via Telegram) and land live on every device; the app is about reviewing and understanding, not data entry. Everything runs on the owner's server.

## Operating Context

- Transactions come from email parsing, receipt photos, recurring rules, or manual entry; low-confidence ones are flagged for Review, and corrections can become category rules.
- Asset balances named like a transaction account (e.g. `Jago`) follow that account's transactions; investment orders arrive pending and settle from later emails.
- Indonesian UI, IDR as the base currency, foreign currencies converted at the day's rate, Asia/Jakarta time.
- Telegram notifications and daily database backups run alongside.

## Capabilities and Constraints

Features that must stay (a redesign may move them, never remove them): transactions in/out/transfer with categories and accounts, search across months, Excel export; multi-currency; monthly overall and per-category budget with spendable remainder, daily allowance, and month-position marker; recurring transactions (weekly by weekday, monthly by month, yearly; auto or Catat/Lewati); assets and portfolio (bank, e-wallet, forex, cash, stocks, bonds, SBN, mutual funds, gold in grams, crypto) with net worth, allocation, gain/loss vs cost, savings goals, return projections, history and pending orders; calendar of daily spending; 6-month analytics; Review queue and category rules; realtime updates; amount-hiding eye toggle; light and dark following the system.

Stack: Svelte 5 + Vite PWA embedded in a single Go binary; no UI component library.

## Brand Commitments

- Name: Kantong. Existing app icon and logo (`docs/logo.png`, `apps/web/public/`).
- The owner chose to look like a typical personal-finance app, at the craft level of Money Lover, Monarch / Copilot, and Indonesian bank apps (Jago, BCA). Familiar patterns are the commitment, not a fallback.

## Evidence on Hand

Real data lives only on the owner's server. README screenshots in `docs/screenshots/` use sample data. No testimonials or public claims exist or are needed.

## Product Principles

1. What's left to spend this month is the headline; everything else supports it.
2. Review, don't type: the inbox of uncertain transactions is one tap from anywhere.
3. Money is private: every amount respects the hide toggle.
4. Familiar beats clever: patterns the owner already knows from other finance apps.
5. Phone and desktop are equal citizens.
