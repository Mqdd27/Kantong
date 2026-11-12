// Regenerates demo/data.json (+ export.xlsx) for the GitHub Pages demo:
// boots the real API on a throwaway DB, seeds six months of sample data, and records every GET the PWA makes.
// Run from the repo root: node apps/web/demo/snapshot.mjs   (needs Go + sqlite3)
import { spawn, execFileSync } from 'node:child_process'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const tmp = mkdtempSync(join(tmpdir(), 'kantong-demo-'))
const db = join(tmp, 'demo.db')
const PORT = '18080', TOKEN = 'demo-token-demo-token-demo-token-00'
const BASE = `http://127.0.0.1:${PORT}/api`

// built binary, not `go run`: killing `go run` leaves its child server holding the port
execFileSync('go', ['build', '-o', join(tmp, 'kantong'), '.'], { cwd: join(here, '../../api'), stdio: 'inherit' })
const server = spawn(join(tmp, 'kantong'), [], {
  env: { ...process.env, PORT, DB_PATH: db, API_TOKEN: TOKEN, APP_PASSWORD: 'demo', SESSION_SECRET: TOKEN, HERMES_API_URL: '', TELEGRAM_BOT_TOKEN: '' },
  stdio: 'inherit',
})
process.on('exit', () => server.kill())

async function req(path, method = 'GET', body) {
  const res = await fetch(BASE + path, {
    method,
    headers: { Authorization: `Bearer ${TOKEN}`, ...(body && { 'Content-Type': 'application/json' }) },
    body: body && JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`)
  return res.status === 204 ? null : path.endsWith('.xlsx') ? Buffer.from(await res.arrayBuffer()) : res.json()
}

for (let i = 0; ; i++) {
  try { await fetch(`http://127.0.0.1:${PORT}/healthz`); break } catch { if (i > 120) throw new Error('server did not start') }
  await new Promise((r) => setTimeout(r, 500))
}

// ---- seed ----
let seed = 7
const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
const pick = (a) => a[Math.floor(rnd() * a.length)]
const between = (lo, hi, step = 500) => Math.round((lo + rnd() * (hi - lo)) / step) * step

const now = new Date(Date.now() + 7 * 3600e3) // Jakarta wall clock
const today = now.toISOString().slice(0, 10)
const month0 = now.toISOString().slice(0, 7)
const shift = (m, d) => { const [y, mo] = m.split('-').map(Number); return new Date(Date.UTC(y, mo - 1 + d, 1)).toISOString().slice(0, 7) }
const months = [-5, -4, -3, -2, -1, 0].map((d) => shift(month0, d))
const at = (m, day, h = 12, min = 0) => `${m}-${String(day).padStart(2, '0')}T${String(h).padStart(2, '0')}:${String(min).padStart(2, '0')}:00+07:00`

const DAILY = [
  ['Makan', ['Warteg Bahari', 'Nasi Padang Sederhana', 'Bakmi GM', 'Sate Khas Senayan', 'Warung Tegal Mbak Sri'], 15000, 55000, 0.9],
  ['Makan', ['GoFood', 'GrabFood', 'ShopeeFood'], 40000, 95000, 0.35],
  ['Jajan', ['Kopi Kenangan', 'Fore Coffee', 'Indomaret', 'Alfamart', 'Janji Jiwa'], 18000, 65000, 0.6],
  ['Transport', ['Gojek', 'Grab', 'KRL Commuter Line', 'MRT Jakarta', 'Pertamina'], 4000, 60000, 0.7],
  ['Belanja', ['Tokopedia', 'Shopee', 'Uniqlo', 'Superindo'], 75000, 450000, 0.12],
  ['Hiburan', ['CGV Grand Indonesia', 'XXI', 'Timezone'], 50000, 150000, 0.05],
  ['Kesehatan', ['Apotek K-24', 'Kimia Farma', 'Halodoc'], 30000, 250000, 0.04],
]
let ref = 1000
const txs = []
for (const m of months) {
  const last = m === month0 ? Number(today.slice(8)) : 28
  txs.push({ occurred_at: at(m, 1, 9), amount: 12500000, type: 'in', category: 'Gaji', merchant: 'PT Nusantara Digital', account: 'BCA' })
  if (2 <= last) txs.push({ occurred_at: at(m, 2, 8), amount: 2500000, type: 'out', category: 'Tagihan', merchant: 'Kos Setiabudi', account: 'BCA' })
  if (3 <= last) txs.push({ occurred_at: at(m, 3, 10), amount: 1000000, type: 'out', category: 'Investasi', merchant: 'Bibit', account: 'BCA', note: 'RDPU bulanan' })
  if (3 <= last) txs.push({ occurred_at: at(m, 3, 10, 30), amount: 1500000, type: 'transfer', category: 'Transfer', merchant: 'Tabungan', account: 'BCA', to_account: 'Bank Jago' })
  if (5 <= last) txs.push({ occurred_at: at(m, 5, 19), amount: 350000, type: 'out', category: 'Tagihan', merchant: 'IndiHome', account: 'BCA' })
  if (7 <= last) txs.push({ occurred_at: at(m, 7, 20), amount: 200000, type: 'out', category: 'Tagihan', merchant: 'Token PLN', account: 'BCA' })
  if (10 <= last) txs.push({ occurred_at: at(m, 10, 7), amount: 0, currency: 'USD', orig_amount: 11.99, type: 'out', category: 'Hiburan', merchant: 'Spotify', account: 'BCA' })
  if (15 <= last && rnd() < 0.5) txs.push({ occurred_at: at(m, 15, 13), amount: between(150000, 400000, 1000), type: 'in', category: 'Transfer Masuk', merchant: 'Split bill teman', account: 'BCA' })
  for (let d = 1; d <= last; d++)
    for (const [category, names, lo, hi, p] of DAILY)
      if (rnd() < p) {
        const email = rnd() < 0.5
        txs.push({
          occurred_at: at(m, d, 7 + Math.floor(rnd() * 14), Math.floor(rnd() * 60)), amount: between(lo, hi), type: 'out', category,
          merchant: pick(names), account: pick(['BCA', 'BCA', 'Bank Jago', 'Tunai']),
          ...(email && { source: 'email', source_ref: `<demo-${ref++}@bca.co.id>`, confidence: 0.95 }),
        })
      }
}
// a few email imports waiting in the Review queue
const lastDay = Number(today.slice(8))
txs.push(
  { occurred_at: at(month0, Math.max(1, lastDay - 1), 21, 14), amount: 87000, type: 'out', category: 'Lainnya', merchant: 'QRIS TOKO JAYA ABADI', account: 'BCA', source: 'email', source_ref: '<demo-r1@bca.co.id>', confidence: 0.55, needs_review: true },
  { occurred_at: at(month0, lastDay, 9, 2), amount: 500000, type: 'transfer', category: 'Transfer', merchant: 'BI-FAST ANDI PRATAMA', account: 'BCA', source: 'email', source_ref: '<demo-r2@bca.co.id>', confidence: 0.9 },
)
for (const t of txs) await req('/transactions', 'POST', { source: 'manual', ...t })

for (const [pattern, type, category] of [['Gojek', 'out', 'Transport'], ['Indomaret', 'out', 'Jajan'], ['Tokopedia', 'out', 'Belanja'], ['Kopi Kenangan', 'out', 'Jajan']])
  await req('/rules', 'POST', { pattern, type, category })

for (const m of months)
  for (const [category, amount] of [['', 7500000], ['Makan', 2000000], ['Jajan', 700000], ['Transport', 800000], ['Belanja', 1000000], ['Hiburan', 400000]])
    await req('/budget', 'PUT', { month: m, category, amount })

const next = (day) => (Number(today.slice(8)) < day ? `${month0}-${day}` : `${shift(month0, 1)}-${day}`)
for (const r of [
  { merchant: 'Kos Setiabudi', category: 'Tagihan', amount: 2500000, day: '02' },
  { merchant: 'IndiHome', category: 'Tagihan', amount: 350000, day: '05' },
  { merchant: 'Token PLN', category: 'Tagihan', amount: 200000, day: '07' },
  { merchant: 'Netflix', category: 'Hiburan', amount: 120000, day: '18' },
  { merchant: 'BPJS Kesehatan', category: 'Kesehatan', amount: 150000, day: '25' },
]) await req('/recurring', 'POST', { type: 'out', category: r.category, merchant: r.merchant, account: 'BCA', amount: r.amount, currency: 'IDR', freq: 'monthly', mask: 0, next_date: next(r.day), auto: false })

// assets last, so the seeded transactions don't move their balances
for (const a of [
  { name: 'BCA', kind: 'bank', currency: 'IDR', balance: 8450000 },
  { name: 'Bank Jago', kind: 'bank', currency: 'IDR', balance: 14200000, target: 25000000, target_date: `${shift(month0, 8)}-01`, note: 'Dana darurat' },
  { name: 'Tunai', kind: 'cash', currency: 'IDR', balance: 650000 },
  { name: 'Wise USD', kind: 'forex', currency: 'USD', balance: 420 },
  { name: 'RDPU Sucorinvest', kind: 'mutual_fund', currency: 'IDR', balance: 18350000, invested: 17500000, return_pct: 5.2 },
  { name: 'ORI025T3', kind: 'sbn', currency: 'IDR', balance: 10000000, invested: 10000000, face: 10000000, return_pct: 6.25, payout: 1, maturity: `${shift(month0, 28)}-15` },
  { name: 'BBCA', kind: 'stock', currency: 'IDR', balance: 9150000, invested: 8700000, note: '1 lot' },
  { name: 'Emas Antam', kind: 'gold', currency: 'XAU', balance: 5, note: 'Logam mulia 5 × 1 gram' },
]) await req('/assets', 'POST', a)

// net-worth history: one point per day, a steady climb with a little noise
// rates come from frankfurter / gold-api: retry so a network blip doesn't record a Rp0 asset
let totalNow
for (let i = 0; ; i++) {
  const p = await req('/assets')
  if (!p.missing_rates.length) { totalNow = p.total_idr; break }
  if (i === 4) throw new Error('missing rates: ' + p.missing_rates)
  await new Promise((r) => setTimeout(r, 3000))
}
const rows = []
for (let i = 180; i >= 1; i--) {
  const d = new Date(Date.parse(today) - i * 864e5).toISOString().slice(0, 10)
  const total = Math.round(totalNow * (1 - i * 0.0011) + (rnd() - 0.5) * 400000)
  rows.push(`('${d}', ${total}, ${Math.round(total * 0.93)})`)
}
execFileSync('sqlite3', [db, `INSERT OR REPLACE INTO asset_snapshots (date, total_idr, invested_idr) VALUES ${rows.join(',')};`])

// ---- capture ----
const data = { base: month0, routes: {} }
const get = async (p) => (data.routes[p] = await req(p))
await get('/categories')
await get('/rules')
await get('/recurring')
const p = await get('/assets')
await get('/assets/history')
for (const a of p.assets) await get(`/assets/${a.id}/activity`)
for (let d = -12; d <= 0; d++) {
  const m = shift(month0, d)
  await get(`/transactions?month=${m}`)
  await get(`/summary?month=${m}`)
  await get(`/budget?month=${m}`)
}
writeFileSync(join(here, 'data.json'), JSON.stringify(data))
writeFileSync(join(here, 'export.xlsx'), await req('/export.xlsx'))
console.log(`demo data: ${txs.length} transactions, ${Object.keys(data.routes).length} routes`)
process.exit(0)
