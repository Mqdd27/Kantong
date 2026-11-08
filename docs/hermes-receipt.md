# Catat transaksi dari foto struk lewat Hermes

Foto struk belanja (Indomaret, Alfamart, restoran, SPBU, bukti QRIS), kirim ke Hermes di Telegram, dan transaksi keluarnya langsung tercatat di Kantong. Hermes membalas apa yang dicatat, misalnya `✓ Keluar Rp27.500 · Jajan · Indomaret · Cash`.

Yang perlu disiapkan sama dengan [hermes-assets.md](hermes-assets.md): Hermes tersambung ke Telegram dengan model yang bisa membaca gambar, plus `FINANCE_API_URL` dan `FINANCE_API_TOKEN` di `~/.hermes/.env`.

## Pasang

Dari folder repo Kantong di server:

```sh
cp -R hermes/finance-receipt ~/.hermes/skills/
python3 ~/.hermes/skills/finance-receipt/scripts/receipt.py   # harus mencetak kategori dan aturan
```

Restart Hermes kalau skill baru belum dikenali.

## Pakai

Kirim fotonya. Boleh ditambah keterangan, misalnya "bayar pakai Jago" kalau metode bayarnya tidak tercetak di struk.

Yang diisi Hermes:

| Field | Dari struk |
|---|---|
| Jumlah | Total yang dibayar (setelah diskon, termasuk pajak), bukan uang tunai yang diserahkan atau kembalian |
| Tanggal | Tanggal & jam di struk; kalau tidak ada, waktu sekarang |
| Merchant | Nama toko tanpa kode cabang |
| Kategori | Dari [aturan](../README.md#fitur) kalau merchant-nya sudah punya aturan, kalau tidak Hermes memilih yang paling cocok |
| Akun | Metode bayar di struk (Tunai → Cash, Debit BCA → BCA, QRIS…); kalau tidak tertera, Cash |
| Catatan | Sampai 5 barang utama |

- Struk yang sama terkirim dua kali tidak tercatat dobel; Hermes membalas "sudah tercatat".
- Kalau struknya buram, transaksi tetap dicatat tapi masuk antrian **Review** di app.
- Salah kategori? Koreksi di app dengan opsi "Selalu kategorikan …", supaya struk berikutnya dari toko itu langsung benar.
- Nomor kartu, nomor member, dan nama kasir tidak disimpan.
