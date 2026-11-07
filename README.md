# Money Manager

Satu binary Go: API + SQLite + SSE realtime + PWA (di-embed). Kontrak API lengkap ada di doc "Money Manager — Prompt per Tahap".

## Jalanin

```sh
cp .env.example .env        # isi tiap secret: openssl rand -hex 32
docker compose up -d --build
curl http://127.0.0.1:8080/healthz   # -> ok
```

Tanpa Docker (dev):

```sh
set -a; . ./.env; set +a
DB_PATH=./money.db go run .
go test ./...
```

Env: `PORT` (8080), `DB_PATH` (/data/money.db), `API_TOKEN`, `APP_PASSWORD`, `SESSION_SECRET` (min 32 karakter), `TZ=Asia/Jakarta`.

## Contoh curl

```sh
API=http://127.0.0.1:8080
AUTH="Authorization: Bearer $API_TOKEN"

# simpan transaksi (201 + id; 409 + id kalau source_ref udah ada)
curl -s -H "$AUTH" -X POST $API/api/transactions -d '{
  "occurred_at":"2026-11-07T12:30:00+07:00","amount":25000,"type":"out","category":"Jajan",
  "merchant":"Indomaret","account":"BCA","note":"","source":"email","source_ref":"<abc@bca>",
  "confidence":0.92,"needs_review":false}'

# cek Message-ID mana yang belum masuk
curl -s -H "$AUTH" -X POST $API/api/source-refs/check -d '{"refs":["<abc@bca>","<new@bca>"]}'

curl -s -H "$AUTH" "$API/api/transactions?month=2026-11"
curl -s -H "$AUTH" "$API/api/summary?month=2026-11"
curl -s -H "$AUTH" $API/api/categories

curl -s -H "$AUTH" -X PATCH $API/api/transactions/1 -d '{"category":"Makan","needs_review":false}'
curl -s -H "$AUTH" -X DELETE $API/api/transactions/1

curl -s -H "$AUTH" -X POST $API/api/rules -d '{"pattern":"INDOMARET","match":"contains","type":"out","category":"Jajan"}'
curl -s -H "$AUTH" $API/api/rules
curl -s -H "$AUTH" -X DELETE $API/api/rules/1

# stream realtime (biarin kebuka, POST di terminal lain)
curl -N -H "$AUTH" $API/api/events

# login PWA (cookie sesi 90 hari)
curl -i -X POST $API/api/login -d "{\"password\":\"$APP_PASSWORD\"}"
```

## Backup

DB-nya satu file di volume `money-data`. Salinan konsisten tanpa stop container:

Image-nya distroless (nggak ada shell/sqlite3), jadi backup dari host (Linux):

```sh
sqlite3 "$(docker volume inspect -f '{{.Mountpoint}}' kantong_money-data)/money.db" \
  "VACUUM INTO 'money-$(date +%F).db'"
```

Di Mac (colima) mountpoint volume ada di dalam VM, jadi lebih gampang: `docker run --rm -v kantong_money-data:/data -v "$PWD":/out alpine sh -c 'apk add -q sqlite && sqlite3 /data/money.db "VACUUM INTO \"/out/money-$(date +%F).db\""'`.

Restore: stop container, ganti `money.db` di volume dengan file backup, hapus `money.db-wal`/`money.db-shm`, start lagi.

## Expose lewat Tailscale (HTTPS)

```sh
tailscale serve --bg 8080        # cek sintaks persisnya: tailscale serve --help
tailscale serve status           # nampilin https://<nama-server>.<tailnet>.ts.net
```

Port cuma di-bind ke `127.0.0.1`, jadi satu-satunya jalan masuk dari luar server adalah Tailscale.
