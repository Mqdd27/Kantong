# Update aset dari screenshot lewat Hermes

Kirim screenshot portofolio (Bibit, Stockbit, Pegadaian, m-banking) ke Hermes di Telegram. Hermes membaca angkanya, menunjukkan ringkasan, lalu membuat atau meng-update aset di Kantong setelah kamu balas **ok**.

```
Screenshot ──Telegram──▶ Hermes (LLM baca gambar)
                            │ assets.py      → GET /api/assets (daftar aset sekarang)
                            ▼
                     ringkasan ke kamu ──"ok"──▶ assets.py apply → PATCH/POST /api/assets
```

Yang perlu disiapkan: Hermes yang sudah tersambung ke Telegram, dengan model yang bisa membaca gambar, dan env `FINANCE_API_URL` + `FINANCE_API_TOKEN` di `~/.hermes/.env`. Kalau kamu sudah mengikuti [hermes-email.md](hermes-email.md), env ini sudah ada.

## 1. Pasang skill

Dari folder repo Kantong di server:

```sh
cp -R hermes/finance-assets ~/.hermes/skills/
python3 ~/.hermes/skills/finance-assets/scripts/assets.py   # harus mencetak daftar aset, satu JSON per baris
```

Kalau Hermes belum mengenali skill baru, restart Hermes.

## 2. Pakai

1. Kirim screenshot ke Hermes, misalnya dengan teks "update aset dari ini".
2. Hermes membalas ringkasan: aset mana yang **baru** dan mana yang **update**, beserta nilainya.
3. Cek angkanya, lalu balas **ok**. Hermes mengirim hasil per aset (`✓ update PBS030`, `✗ …` kalau gagal).

Aset dicocokkan dari nama. Kalau namanya sama dengan aset yang sudah ada (misalnya `PBS030`), aset itu yang di-update; kalau tidak, dibuat aset baru. Yang di-update hanya field yang terbaca di screenshot.

## Cara Hermes mengisi field

| Produk | Isi di Kantong |
|---|---|
| SBN (FR, PBS, ORI, SR, ST, SBR) | Jenis SBN. Nilai = modal + keuntungan, Modal = Total Modal Investasi, Nominal = jumlah unit × Rp1.000.000, Return = **Kupon / Tahun** (bukan yield), Kupon dibayar tiap 6 bulan (FR/PBS) atau tiap bulan (ORI/SR/ST/SBR), Jatuh tempo |
| Obligasi korporasi | Jenis Obligasi, field sama; tanpa kupon kalau memang tidak ada kupon |
| Reksa dana | Nilai sekarang dan modal |
| Emas | Mata uang **Emas (gram)**: isi jumlah gram, modal dalam IDR. Nilainya ikut harga emas harian |
| Saham | Nilai sekarang dan modal (total) |
| Bank | Saldo, dan bunga % per tahun kalau tertera |

Nomor rekening tidak pernah disimpan. Field yang tidak terbaca jelas dilewati, dan Hermes menyebutkan field mana.

## Troubleshooting

| Gejala | Cek |
|---|---|
| Hermes tidak memakai skill ini | Restart Hermes; pastikan folder `~/.hermes/skills/finance-assets` ada |
| `list failed: 401` | `FINANCE_API_TOKEN` harus sama dengan `API_TOKEN` di `.env` Kantong |
| `✗ … 400 …` | Pesan error menyebut field yang salah (misalnya format tanggal jatuh tempo harus YYYY-MM-DD) |
| Aset jadi dobel | Nama di screenshot beda dengan nama di Kantong. Samakan namanya di Kantong, lalu hapus aset yang dobel |
| Muncul "Interrupting current task" | Hermes sedang menjalankan tugas lain (misalnya cron email). Kirim `/busy queue` sekali supaya pesan berikutnya menunggu |
