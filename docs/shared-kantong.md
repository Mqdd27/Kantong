# Kantong Bersama (KTG-2, issue #24)

Status: desain, belum ada kode. Keputusan produk sudah disetujui pemilik (lihat bagian 1).

## 1. Keputusan produk

- **Satu rekening, banyak pemilik.** Kantong bersama adalah satu baris aset yang dipakai bersama. Saldo dan transaksinya cuma disimpan sekali.
- **Semua anggota bisa edit.** Tambah, ubah, dan hapus transaksi serta saldo di kantong bersama.
- **Transaksi dicatat siapa yang memasukkan** (kolom `created_by`).
- **Undang anggota:** hanya pembuat yang boleh mengundang. Anggota undangan tidak bisa mengundang orang lain.
- **Keluarkan anggota:** pembuat boleh mengeluarkan anggota.
- **Hapus kantong bersama:** hanya pembuat, dengan konfirmasi mengetik nama kantong. Semua transaksi di dalamnya ikut hilang untuk semua anggota.
- **Pembuat akun dihapus:** kepemilikan pindah ke anggota tertua (berdasarkan waktu bergabung). Kalau tidak ada anggota lain, kantong dihapus.
- **Net worth dihitung sekali.** Di tampilan tiap anggota, saldo kantong bersama masuk satu kali ke total. Kalau nanti ada tampilan gabungan rumah tangga, kantong bersama juga cuma dihitung sekali di situ. Pengeluaran dari kantong bersama juga cuma dihitung sekali di pengeluaran bulanan (per transaksi, bukan per anggota).

## 2. Skema (migrasi 023)

```sql
CREATE TABLE shared_assets (            -- satu baris = satu kantong bersama
  id          INTEGER PRIMARY KEY,
  asset_id    INTEGER NOT NULL UNIQUE REFERENCES assets (id) ON DELETE CASCADE,
  created_by  INTEGER NOT NULL REFERENCES users (id)
);
CREATE TABLE shared_members (
  shared_id   INTEGER NOT NULL REFERENCES shared_assets (id) ON DELETE CASCADE,
  user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role        TEXT NOT NULL CHECK (role IN ('owner', 'member')),
  joined_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  PRIMARY KEY (shared_id, user_id)
);
ALTER TABLE transactions ADD COLUMN created_by INTEGER REFERENCES users (id);
```

Catatan:
- `assets` tetap punya `user_id`, tetapi untuk aset bersama artinya "pembuat" (`created_by` dipakai sebagai sumber kebenaran pemilik).
- Kantong bersama **dikecualikan** dari `UNIQUE (user_id, name)`. Nama tetap unik per grup dengan cek aplikasi. Aset pribadi milik anggota dengan nama sama harus ditolak saat membuat kantong bersama (atau sebaliknya), supaya tidak ambigu.
- Rebuild tabel `assets` mengikuti pola migrasi 021 kalau constraint berubah.

## 3. Perubahan perilaku yang kritis

| Area | Sekarang | Setelah fitur |
|---|---|---|
| Scope data | `WHERE user_id = ?` | Untuk aset: `user_id = ? OR asset_id IN (member kantong)`. Untuk transaksi: transaksi dari siapa pun yang akun/tujuannya cocok dengan kantong bersama yang dia anggotai. |
| Saldo live (`liveBalance`) | Hanya transaksi `user_id` pemilik | Harus menjumlahkan transaksi dari **semua anggota** untuk aset bersama. Satu fungsi terpusat, dipakai portfolio, budget, dan snapshot. |
| Snapshot | Per user | Snapshot kantong bersama disimpan per grup agar tidak dobel saat tiap anggota membuka halaman. |
| Email/Hermes | Transaksi masuk ke akun yang login | Transaksi yang akunnya cocok dengan kantong bersama dicatat ke kantong itu, dengan `created_by` = pemilik mailbox. |
| Telegram | Per akun | Tiap anggota menerima notifikasi perubahan kantong bersama. Yang mencatat tidak menerima notifikasi untuk dirinya sendiri. (Bisa diubah kalau tidak diinginkan.) |

**Catatan penting:** sekarang saldo dihitung dengan mencocokkan **nama akun** pada transaksi. Untuk kantong bersama, kecocokan nama harus dibatasi ke anggota grup itu saja, supaya dua orang di luar grup yang kebetulan punya aset bernama sama tidak ikut terhitung.

## 4. Endpoint baru

| Method | Path | Siapa | Fungsi |
|---|---|---|---|
| POST | `/api/shared` | Semua user | Buat kantong bersama dari aset pribadi atau baru |
| GET | `/api/shared` | Anggota | Daftar kantong bersama + anggota + peran |
| POST | `/api/shared/{id}/members` | Pembuat | Undang user (by username) |
| DELETE | `/api/shared/{id}/members/{uid}` | Pembuat | Keluarkan anggota |
| DELETE | `/api/shared/{id}` | Pembuat | Hapus kantong (body: `{"confirm": "<nama>"}`) |
| POST | `/api/shared/{id}/leave` | Anggota | Keluar dari kantong (bukan pembuat) |

Transaksi dan aset bersama memakai endpoint yang sudah ada, dengan pengecekan keanggotaan. Pengecekan ini ditulis sebagai satu helper (`canAccessAsset(uid, assetID, write bool)`), jangan diulang per handler.

Aturan `leave`: anggota yang keluar kehilangan akses, tetapi transaksi yang sudah dia buat tetap di kantong (`created_by` tetap menunjuk dia).

## 5. Rencana tes (tulis dulu, sebelum implementasi)

Tambahkan ke `isolation_test.go`:
1. User lain di luar grup tidak bisa baca, ubah, atau hapus kantong bersama (404, bukan 403, supaya keberadaannya tidak bocor).
2. Anggota bisa baca dan tulis transaksi serta saldo.
3. Anggota undangan tidak bisa mengundang, mengeluarkan, atau menghapus kantong.
4. Pembuat bisa mengundang, mengeluarkan, dan menghapus (dengan konfirmasi nama yang benar, salah = ditolak).
5. Saldo live menjumlahkan transaksi dari dua anggota tanpa dobel.
6. Net worth per anggota memasukkan kantong bersama tepat sekali.
7. Hapus akun pembuat: kepemilikan pindah ke anggota tertua; tanpa anggota lain kantong terhapus.
8. Nama kantong bersama bentrok dengan aset pribadi anggota ditolak.
9. Transaksi email/Hermes ke akun bersama tercatat dengan `created_by` benar.

## 6. Skill agent (setup, create, delete, update)

- Skill ditaruh di profil Hermes pemilik saja (sama seperti `kantong-onboard`), karena operasi ini memakai token pemilik.
- Skill hanya memanggil endpoint di atas; tidak pernah menulis DB langsung.
- Hapus dan keluarkan anggota selalu minta konfirmasi eksplisit di chat.
- Skill ini sebaiknya dibuat **setelah** API stabil, bukan sebelumnya.

## 7. Urutan implementasi

1. Tes isolasi untuk aturan di atas (merah dulu).
2. Migrasi 023 + rebuild `assets` / `transactions` di salinan DB.
3. Helper akses + `liveBalance` terpusat.
4. Endpoint `/api/shared`.
5. Perubahan jalur email/Hermes/Telegram.
6. UI: tombol "Bersama" di Assets, daftar anggota, undang/keluar/hapus.
7. Skill agent.

Sebelum langkah 2: backup manual `money.db` dan uji migrasi di salinan DB dengan dua akun tes.

## 8. Yang belum diputuskan

- Tampilan gabungan rumah tangga (net worth semua orang dalam satu angka): ikut fitur ini atau nanti?
- Notifikasi Telegram untuk pencatat sendiri: tetap dikirim atau tidak?
- Transaksi yang dibuat anggota lalu dia keluar dari grup: tetap terlihat anggota lain (rekomendasi: ya).

## 9. Keputusan implementasi (dicatat saat membangun, disetujui pemilik di chat)

- **Tampilan net worth gabungan rumah tangga:** nanti, issue terpisah.
- **Notifikasi Telegram:** pencatat tetap menerima notifikasi seperti biasa; anggota lain menerima pesan yang sama ditambah "dicatat <username>". Hanya transaksi dari email, sama seperti sebelumnya.
- **Transaksi anggota yang keluar/dikeluarkan:** tetap di kantong (dipindah ke pemegang kantong, `created_by` tetap dia).
- **Skema:** `transactions` tidak di-rebuild (cukup `ADD COLUMN created_by ... ON DELETE SET NULL`), `assets` tidak di-rebuild. `UNIQUE (user_id, name)` tidak pernah bentrok karena nama kantong unik di antara anggotanya (cek aplikasi, tanpa beda huruf besar/kecil).
- **Pemilik baris transaksi:** `user_id` = pencatat, sama dengan `created_by`. Transaksi masuk kantong kalau dicatat anggota, setelah kantong dibuat (`shared_assets.created_at`), dan akunnya/tujuannya sama dengan nama kantong. Satu definisi: view `kantong_tx`. Kolom `created_at` di `shared_assets` ditambahkan supaya transaksi lama anggota dengan nama akun yang sama tidak bocor.
- **Angka pribadi:** net worth dan budget/ringkasan bulanan tiap anggota memasukkan kantong bersama tepat sekali; di halaman Aset kantong bersama tampil di section sendiri.
- **Snapshot:** tidak ada tabel per grup. Snapshot menyimpan total per akun (bukan per aset), dan total itu memasukkan kantong sekali, jadi tidak bisa dobel.
- **Hapus akun:** belum ada fitur hapus akun. Yang dibuat hanya `handOffShared(uid)` yang dijalankan sebelum akun dihapus nanti.
- **`canAccessAsset(uid, assetID)`:** tanpa parameter `write`, karena semua anggota boleh baca dan tulis. Hapus aset bersama lewat `/api/assets` ditolak (409); hanya lewat `DELETE /api/shared/{id}`.
- **Edit transaksi anggota lain:** boleh, kecuali memindahkan akunnya ke akun lain (400), supaya tidak bisa mengubah saldo aset pribadi orang lain.
- **Hapus kantong:** `source_ref` email dari transaksi yang terhapus dicatat di `deleted_refs`, supaya email cron tidak mengimpornya lagi.
