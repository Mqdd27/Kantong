# Integrasi email bank dengan Hermes

Panduan ini menyambungkan email notifikasi bank/broker ke Kantong lewat agent [Hermes](https://github.com/NousResearch/hermes-agent). Hasil akhirnya: setiap email transaksi otomatis tercatat di Kantong dalam ±10 menit, lalu kamu dapat notif Telegram.

```
Bank/broker ──email──▶ Gmail ──filter forward──▶ mailbox IMAP (iCloud/Gmail)
                                                       │ cron Hermes tiap 10 menit
                                                       ▼
                                        fetch.py (tanpa LLM): ada email baru?
                                          │ tidak → selesai, 0 token
                                          ▼ ya
                                  Hermes (LLM) parsing → post.py → Kantong API → Telegram + PWA
```

Yang perlu disiapkan:

- Server yang menjalankan Kantong dan Hermes (bisa server yang sama).
- Mailbox yang bisa diakses lewat IMAP pakai **app password** (iCloud Mail atau Gmail).
- Bot Telegram yang sudah tersambung ke Hermes.

## 1. Siapkan mailbox

Skill membaca email secara **read-only**: email tidak ditandai dibaca, tidak dipindah, dan tidak dihapus. Kamu boleh pakai mailbox utama atau mailbox khusus.

**Buat app password** (password akun biasa tidak bisa dipakai untuk IMAP):

| Provider | Host IMAP | Cara bikin app password |
|---|---|---|
| iCloud Mail | `imap.mail.me.com` | appleid.apple.com → Sign-In and Security → App-Specific Passwords |
| Gmail | `imap.gmail.com` | Aktifkan 2-Step Verification → myaccount.google.com/apppasswords |

**Kalau email bank masuk ke Gmail tapi skill membaca mailbox lain** (misalnya iCloud), forward otomatis dari Gmail:

1. Buka Gmail → Settings → Forwarding and POP/IMAP → Add a forwarding address, lalu isi alamat mailbox tujuan.
2. Konfirmasi lewat email dari `forwarding-noreply@google.com`. Skill otomatis melewati email ini.
3. Bikin filter: Settings → Filters → Create a new filter. Isi kolom **From** dengan alamat pengirim bank, misalnya `noreply@jago.com OR bca@bca.co.id OR no-reply@bibit.id`, lalu centang **Forward it to** alamat tadi.

Email bank yang sempat kelewat bisa juga di-**Fwd manual** dari Gmail. Skill mengenali forward yang dikirim dari `FINANCE_FWD_FROM` dengan subjek berawalan `Fwd:`, lalu mengambil pengirim aslinya dari blok "Forwarded message".

## 2. Pasang skill

Jalankan dari folder repo Kantong di server:

```sh
cp -R hermes/finance-email ~/.hermes/skills/
cp hermes/finance-email-fetch.py ~/.hermes/scripts/   # cron Hermes cuma boleh jalanin script dari folder ini
```

## 3. Isi env

Tambahkan ke `~/.hermes/.env`. Jangan tulis nilai-nilai ini di file skill:

```sh
IMAP_HOST=imap.mail.me.com
IMAP_USER=alamat-mailbox@icloud.com
IMAP_PASSWORD=app-password-dari-langkah-1

FINANCE_API_URL=http://127.0.0.1:8080      # alamat Kantong dilihat dari server Hermes
FINANCE_API_TOKEN=isi-sama-dengan-API_TOKEN-di-.env-Kantong

FINANCE_FWD_FROM=gmail-kamu@gmail.com      # alamat yang dipakai untuk Fwd manual (boleh kosong)
FINANCE_OWNER_NAME=NAMA LENGKAP            # sesuai yang tertulis di bukti transfer, buat deteksi transfer ke rekening sendiri
```

Supaya Kantong juga mengirim notif Telegram, isi `TELEGRAM_BOT_TOKEN` dan `TELEGRAM_CHAT_ID` di `.env` **Kantong**, lalu jalankan `docker compose up -d --force-recreate`.

## 4. Atur pengirim yang dibaca

Daftar pengirim ada di `ALLOW` pada `scripts/fetch.py`, dan aturan parsing tiap bank ada di `parsing-rules.md`. Bawaannya mencakup Jago, BCA, SeaBank, Bibit, Stockbit, dan Krom.

Untuk menambah bank:

1. Tambahkan alamat pengirimnya ke `ALLOW`.
2. Tambahkan bagian baru di `parsing-rules.md`: contoh subjek, letak nominal, letak waktu, dan apakah transaksinya `in`, `out`, atau `transfer`. Sertakan juga potongan contoh email **tanpa nomor rekening atau saldo**.

Paling cepat, minta Hermes yang menulis aturannya. Kirim lewat chat:

> Ambil 5 email terakhir dari `<alamat bank>` (read-only), parse jadi transaksi sesuai format Kantong, lalu tambahkan aturannya ke `~/.hermes/skills/finance-email/parsing-rules.md`.

Cek hasilnya. Kalau ada yang salah, koreksi Hermes lalu ulangi.

## 5. Tes manual

```sh
python3 ~/.hermes/skills/finance-email/scripts/fetch.py
```

Baris terakhir output-nya berupa JSON:

- `{"wakeAgent": false}` artinya tidak ada email baru. Ini normal kalau semua email sudah tercatat.
- `{"wakeAgent": true, "data": {"emails": [...]}}` artinya ada email baru yang siap di-parse.

Kalau ada error IMAP atau API, lihat log-nya:

```sh
tail ~/.hermes/finance-email/finance-email.log
```

## 6. Jadwalkan cron

```sh
hermes cron create --name finance-email --script finance-email-fetch.py --skill finance-email --deliver telegram 10m
hermes cron list
```

Setiap 10 menit, `fetch.py` jalan duluan tanpa LLM. Hermes cuma dibangunkan kalau ada email yang `Message-ID`-nya belum tercatat di Kantong, jadi run tanpa email baru tidak makan token.

## 7. Cek end-to-end

1. Lakukan transaksi kecil, misalnya top-up e-wallet Rp10.000.
2. Tunggu sampai 10 menit. Notif Telegram `Transfer Rp10.000 · …` akan masuk, dan transaksinya muncul di PWA tanpa perlu refresh.
3. Kalau kategorinya salah, tap kartu kuning **"… transaksi perlu direview"** di Beranda PWA (di desktop: menu **Review** di sidebar), lalu tap kategori yang benar dengan opsi "Ingat pilihan" menyala. Email berikutnya dari merchant itu langsung masuk dengan kategori tersebut.

Mau mengisi aset dari screenshot portofolio juga? Lihat [hermes-assets.md](hermes-assets.md).

## Troubleshooting

| Gejala | Cek |
|---|---|
| `FETCH config missing: …` di log | Env di `~/.hermes/.env` belum lengkap |
| IMAP `AUTHENTICATIONFAILED` | Yang dipakai harus app password, bukan password akun |
| Email bank tidak pernah kebaca | Alamat pengirim belum ada di `ALLOW`, atau filter forward Gmail belum jalan (cek apakah emailnya sampai ke mailbox IMAP). Folder yang dibaca hanya `INBOX` dan `Junk`, untuk 3 hari terakhir |
| Transaksi masuk dua kali | Tidak mungkin dari email yang sama, karena `source_ref` (Message-ID) unik. Kalau dobel dengan transaksi berulang, matikan "Catat otomatis" di transaksi berulang itu |
| Kategori sering salah | Koreksi di Review supaya jadi aturan, atau perbaiki `parsing-rules.md` |
| Tidak ada notif Telegram | `TELEGRAM_BOT_TOKEN`/`TELEGRAM_CHAT_ID` di `.env` Kantong, lalu `docker logs kantong-money-1 \| grep telegram` |
