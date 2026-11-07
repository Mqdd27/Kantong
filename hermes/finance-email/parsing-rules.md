# Finance email parsing rules (v0 — based on 1 real sample)

Status: TEST phase, read-only. Only Jago "Kamu telah membayar" has been verified against a real email.
All other senders below are UNVERIFIED (no sample in mailbox yet) — do not trust them until a sample is seen.

## Access
- IMAP iCloud, env IMAP_HOST / IMAP_USER / IMAP_PASSWORD from ~/.hermes/.env
- `select(folder, readonly=True)`, fetch with `BODY.PEEK[]`. Never store/flag/copy/move/delete/expunge.
- Folders: INBOX and Junk (Junk was empty at test time). Mailbox also has Archive, Icloud, etc. — not scanned.
- Note: `search(None,'FROM',x)` can return None data on iCloud for an empty folder; guard for it. Safer: search ALL, filter locally.

## Sender matching
1. Direct: header From address in the allowlist.
2. Manual forward: From is `FINANCE_FWD_FROM` AND Subject starts with "Fwd:".
   Take the original sender from the body line `From: Name <addr>` right after `---------- Forwarded message ---------`
   (use the text/plain part; HTML has the same block with &lt;addr&gt;). Match addr to the allowlist.
3. Original time = body line `Date:` of the forward block is Gmail-localised ("Thu, Nov 5, 2026 at 12:42 PM") — prefer the
   transaction time inside the email body; fall back to this Date; last resort the iCloud Date header.
4. Forward block `To:` is the user's Gmail — ignore.

Allowlist: noreply@jago.com, no-reply@stockbit.com, noreply@krom.id, bca@bca.co.id, pasporbca@klikbca.com, no-reply@bibit.id, alerts@seabank.co.id

## Always skip (non-transaction)
- Gmail Forwarding Confirmation (forwarding-noreply@google.com)
- Empty/test forwards without "Fwd:" and without a forwarded block
- Promo, OTP, login info, monthly e-statement/laporan

## Output
Fields: occurred_at, amount, type, category, merchant, account, note, source="email", source_ref (Message-ID with <>), confidence, needs_review (confidence<0.7).
- amount: int rupiah, positive. Strip "Rp", thousand dots, ",00". No amount found -> skip.
- occurred_at ISO 8601 with +07:00 ("WIB").
- Never output account/card numbers (including PAN, terminal ID) or balances. Do not put them in `note`.
- type: in / out / transfer (own accounts: top-up, tarik tunai, RDN Stockbit/Bibit). Own name: `owner_name` in the fetch payload (from `FINANCE_OWNER_NAME`).

## Jago (noreply@jago.com) — VERIFIED
Subject: `Kamu telah membayar ke <Merchant> <store no.>` (outgoing payment, QRIS-style).
Body (text/plain), label on one line, value after a blank line:
- `Dari` -> source account number (do NOT output)
- `Ke` -> merchant name, then merchant ID number (ignore the number)
- `Jumlah` -> `Rp 26.000`
- `Tanggal Transaksi` -> `05 November 2026, 12:42 WIB` (Indonesian month names)
- `Status Transaksi` -> must be `Berhasil`, else skip
- `Jumlah Tip` -> ignore unless > 0 (then add to amount only if clearly part of the debit; flag needs_review)
Mapping: type=out, account=Jago. Merchant: strip trailing store number (`Kopi Kenangan 1320` -> `Kopi Kenangan`).
Category: coffee/food/drink outlet -> Jajan (Makan only for full meals); ambiguous -> confidence ~0.75.
Other Jago subjects to expect (unverified): transfer in ("menerima"), transfer out to a person; "Kamu baru saja mengirimkan uang" in the body is generic text for all outgoing.

## Stockbit, Bibit, Krom, BCA (bca@bca.co.id, pasporbca@klikbca.com), SeaBank — UNVERIFIED
No sample seen. When first sample appears, record: subject pattern, label names, date format, amount format here.
Planned defaults (from user spec, not from emails):
- Beli reksadana/saham/obligasi -> out / Investasi; setor/tarik RDN or top-up own account -> transfer / Transfer.
- BCA/KlikBCA transfer to person -> out / Lainnya unless merchant is clear; incoming -> in / Transfer Masuk.
- Account name = Jago, BCA, SeaBank, Stockbit, Bibit, Krom by sender.

## Foreign currency (any sender)
- Card/online charge in another currency (Claude/Anthropic, OpenAI, Apple, Google, etc. in USD; forex pocket spend in SGD/JPY/CNY/KRW...):
  `currency` = ISO code, `orig_amount` = the foreign amount as a number (dot decimal: "USD 20.00" -> 20.0).
- If the email also shows the IDR debited (kurs/"setara"/"Rp ..."), put it in `amount` (int rupiah). Otherwise omit `amount`.
- Subscription to Claude/Anthropic -> out / Tagihan, merchant "Claude".
- Buying foreign currency into an own forex pocket (IDR -> SGD etc.) -> type transfer / Transfer, amount = IDR debited.

## Investment orders (Bibit, Stockbit, SBN via any app) -> `orders`, not `transactions`
One object per email: `{asset, kind, currency, side, amount, status, order_ref, source_ref, occurred_at, note}`
- `asset`: product name as shown (e.g. "ORI027T3", "SR021T5", "Obligasi FR0100", "Reksa Dana Sucorinvest Money Market"); reuse the exact
  name from `data.assets` if it is the same product.
- `kind`: sbn (ORI, SR, ST, SBR, SWR), bond (FR, corporate bonds/obligasi), mutual_fund (reksa dana), stock, gold, crypto, other.
- Stocks: the user tracks only the TOTAL stock portfolio, not single tickers. Never create per-ticker assets; skip stock buy/sell
  confirmation emails (put them in `skip`).
- `side`: buy / sell. `amount`: order value in the asset currency (number, rupiah for IDR). `currency` default IDR.
- `status`: "pending" for konfirmasi/pesanan diterima/menunggu pembayaran; "success" for berhasil/selesai/dialokasikan;
  "failed" for gagal/dibatalkan/kedaluwarsa.
- `order_ref`: the order/transaction number in the email (e.g. "No. Transaksi"). Same order across emails MUST use the same order_ref.
  No order number -> use `<asset>|<side>|<amount>|<date YYYY-MM-DD>` so the confirmation and the result email produce the same ref.
- `source_ref`: Message-ID with <>.
- Order emails are UNVERIFIED (no sample yet); set `note` to the email subject for traceability.
