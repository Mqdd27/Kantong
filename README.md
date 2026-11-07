# Kantong

Money manager pribadi yang self-hosted. Notifikasi transaksi dari email bank/broker dibaca otomatis oleh agent, disimpan lewat API, dan langsung muncul realtime di PWA.

Satu binary Go (API + SQLite + SSE + PWA Svelte yang di-embed), image Docker ±22 MB, RAM idle ±3 MB.

![Beranda](docs/screenshots/home.png)

<p align="center">
  <img src="docs/screenshots/mobile-home.png" width="260" alt="Beranda di HP" />
  <img src="docs/screenshots/mobile-budget.png" width="260" alt="Budget di HP" />
  <img src="docs/screenshots/mobile-assets.png" width="260" alt="Aset di HP" />
</p>

> Semua screenshot pakai data contoh.

## Fitur

- **Transaksi**: masuk / keluar / transfer dengan kategori. Bisa dicari di semua bulan dan di-export ke CSV.
- **Multi-currency**: transaksi dalam USD/SGD/JPY/... disimpan nominal aslinya, lalu dikonversi ke IDR pakai kurs hari itu ([frankfurter.dev](https://frankfurter.dev), data ECB).
- **Budget bulanan**: total dan per kategori, berlaku terus ke bulan berikutnya sampai diubah. Ada sisa bisa dibelanjakan (dikurangi tagihan yang akan datang), jatah per hari, dan penanda posisi bulan.
- **Transaksi berulang**: gaji, langganan, cicilan. Bisa dicatat otomatis atau manual (Catat / Lewati).
- **Aset & portofolio**: rekening, valas, saham (total), obligasi, SBN, reksa dana, emas, kripto. Ada total kekayaan dalam IDR, alokasi per jenis, untung/rugi vs modal, dan target tabungan.
- **Order investasi dari email**: konfirmasi pembelian dicatat *pending*, lalu email berhasil/gagal menyelesaikannya dan meng-update saldo aset.
- **Analitik**: 6 bulan masuk vs keluar, tingkat tabungan, dan kategori bulan ini dibanding rata-ratanya.
- **Review**: transaksi hasil parsing email yang kurang yakin ditandai untuk dicek sekali tap.
- **Realtime**: transaksi baru dari agent langsung muncul di semua perangkat (SSE).
- **Tampilan**: layout terpisah untuk desktop dan mobile, mengikuti dark mode sistem, dan bisa di-install sebagai PWA.

| Budget | Aset |
|---|---|
| ![Budget](docs/screenshots/budget.png) | ![Aset](docs/screenshots/assets.png) |

![Analitik](docs/screenshots/analytics.png)

## Cara kerja

```
Email bank/broker ──▶ mailbox (IMAP, read-only)
                            │  cron tiap 10 menit
                            ▼
                     Agent (LLM) ── parsing ──▶ POST /api/transactions, /api/orders
                                                        │
                                              Go API + SQLite (WAL)
                                                        │ SSE
                                                        ▼
                                                PWA (desktop & HP)
```

- Tahap fetch berjalan tanpa LLM. Email yang `Message-ID`-nya sudah tercatat dilewati lewat `POST /api/source-refs/check`, jadi LLM cuma dibangunkan kalau ada email baru.
- `source_ref` (Message-ID) bersifat unik, sehingga email yang sama nggak pernah tercatat dua kali.
- Transaksi dengan `confidence < 0.7` otomatis masuk antrian Review.

## Jalanin

```sh
cp .env.example .env        # isi tiap secret: openssl rand -hex 32
docker compose up -d --build
curl http://127.0.0.1:8080/healthz   # -> ok
```

| Env | Default | Keterangan |
|---|---|---|
| `API_TOKEN` | – | Bearer token untuk agent/script |
| `APP_PASSWORD` | – | Password login PWA |
| `SESSION_SECRET` | – | Minimal 32 karakter, untuk tanda tangan cookie sesi |
| `HOST_BIND` / `HOST_PORT` | `127.0.0.1` / `8080` | Alamat & port yang di-publish compose |
| `COOKIE_SECURE` | `true` | Set `false` hanya kalau diakses lewat HTTP biasa (tanpa HTTPS, PWA nggak bisa di-install) |
| `FX_URL` | `https://api.frankfurter.dev` | Sumber kurs |
| `DB_PATH` / `PORT` | `/data/money.db` / `8080` | Di dalam container |

Perubahan `.env` baru terpakai setelah `docker compose up -d --force-recreate`.

### Dev

```sh
cd web && npm ci && npm run build && cd ..   # build PWA ke web/dist (di-embed)
set -a; . ./.env; set +a
DB_PATH=./money.db go run .
go test ./...
cd web && npm run dev                        # Vite + proxy /api ke :8080
```

## API

Semua endpoint `/api/*` butuh `Authorization: Bearer $API_TOKEN` atau cookie sesi dari `POST /api/login`.

| Endpoint | Fungsi |
|---|---|
| `GET/POST /api/transactions`, `PATCH/DELETE /api/transactions/{id}` | Transaksi (`?month=YYYY-MM` atau `?q=cari`) |
| `POST /api/source-refs/check` | Message-ID mana yang belum tercatat |
| `GET /api/summary?month=` | Total masuk/keluar per kategori |
| `GET /api/categories`, `GET/POST/DELETE /api/rules` | Kategori & aturan merchant → kategori |
| `GET/PUT /api/budget` | Budget bulan ini vs pengeluaran + tagihan mendatang |
| `GET/POST/PATCH/DELETE /api/recurring`, `POST /api/recurring/{id}/advance` | Transaksi berulang (`?skip=1` untuk lewati) |
| `GET/POST/PATCH/DELETE /api/assets`, `GET /api/assets/history` | Aset, total kekayaan, riwayat harian |
| `POST /api/orders`, `PATCH /api/orders/{order_ref}` | Order investasi pending → success/failed |
| `GET /api/fx?currency=USD` | Kurs ke IDR |
| `GET /api/export.csv` | Semua transaksi |
| `GET /api/events` | Stream SSE |

```sh
API=http://127.0.0.1:8080; AUTH="Authorization: Bearer $API_TOKEN"

# transaksi dari email (201; 409 + id kalau source_ref sudah ada)
curl -s -H "$AUTH" -X POST $API/api/transactions -d '{
  "occurred_at":"2026-11-07T12:30:00+07:00","amount":25000,"type":"out","category":"Jajan",
  "merchant":"Indomaret","account":"BCA","source":"email","source_ref":"<abc@bank>","confidence":0.92}'

# tagihan USD: amount IDR dihitung dari kurs hari itu kalau dikosongkan
curl -s -H "$AUTH" -X POST $API/api/transactions -d '{
  "type":"out","category":"Tagihan","merchant":"Claude","source":"manual","currency":"USD","orig_amount":20}'

# order SBN: pending dulu, lalu diselesaikan dari email berikutnya
curl -s -H "$AUTH" -X POST $API/api/orders -d '{"asset":"ORI027T3","kind":"sbn","side":"buy","amount":1000000,"order_ref":"TRX-123"}'
curl -s -H "$AUTH" -X PATCH $API/api/orders/TRX-123 -d '{"status":"success"}'

curl -N -H "$AUTH" $API/api/events   # biarin kebuka, lihat event masuk
```

## Backup

Seluruh data ada di satu file SQLite di volume `money-data`. Image-nya distroless (nggak ada shell), jadi backup dijalankan dari container sementara, tanpa perlu stop app:

```sh
docker run --rm -v kantong_money-data:/data -v "$PWD":/out alpine \
  sh -c 'apk add -q sqlite && sqlite3 /data/money.db "VACUUM INTO \"/out/money-$(date +%F).db\""'
```

Restore: stop container, ganti `money.db` di volume dengan file backup, hapus `money.db-wal` dan `money.db-shm`, lalu start lagi.
