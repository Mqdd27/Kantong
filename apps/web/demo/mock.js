// In-browser stand-in for the Go API, used only by `vite build --mode demo` (GitHub Pages).
// Reads come from data.json (recorded by snapshot.mjs); transaction writes live in memory until reload.
import raw from './data.json?raw'

// Move every YYYY-MM so the snapshot's newest month is always "this month" (seeded days stop at 28).
const ym = (m) => { const [y, mo] = m.split('-').map(Number); return y * 12 + mo - 1 }
const now = new Date(Date.now() + 7 * 3600e3).toISOString().slice(0, 7)
const delta = ym(now) - ym(raw.match(/"base":"(\d{4}-\d{2})"/)[1])
const shifted = raw.replace(/\b(20\d\d)-(0[1-9]|1[0-2])\b/g, (_, y, m) => {
  const n = Number(y) * 12 + Number(m) - 1 + delta
  return `${Math.floor(n / 12)}-${String((n % 12) + 1).padStart(2, '0')}`
})
const { routes } = JSON.parse(shifted)

const months = Object.keys(routes).filter((k) => k.startsWith('/transactions?month=')).sort()
const oldest = months[0].slice(-7)
let nextId = 1e6

const byNewest = (a, b) => (a.occurred_at < b.occurred_at ? 1 : -1)
const allTx = () => months.flatMap((k) => routes[k])
function listFor(m) {
  const k = `/transactions?month=${m}`
  if (!routes[k]) months.push(k), (routes[k] = [])
  return routes[k]
}
function find(id) {
  for (const k of months) {
    const i = routes[k].findIndex((t) => t.id === id)
    if (i >= 0) return [routes[k], i]
  }
  return [null, -1]
}
function put(t) {
  const list = listFor(t.occurred_at.slice(0, 7))
  list.push(t)
  list.sort(byNewest)
  return t
}

const reply = 'Ini versi demo, jadi Hermes tidak benar-benar dihubungi. Di Kantong yang kamu jalankan sendiri, jawaban datang dari agent Hermes kamu dan datanya dibaca lewat skill finance-*.'
const chatSession = { id: 1, title: 'Makan bulan ini boros tidak?', updated_at: `${now}-10 20:15:00` }
const chatMessages = [
  { id: 1, role: 'user', text: 'Makan bulan ini boros tidak?', created_at: chatSession.updated_at },
  { id: 2, role: 'assistant', text: 'Masih aman. Makan bulan ini sekitar 40% dari budget Rp2.000.000, dan jatah per harinya masih cukup sampai akhir bulan. Yang paling sering: GoFood dan warteg.', model: 'demo', created_at: chatSession.updated_at },
]

export async function api(path, { method = 'GET', body } = {}) {
  await new Promise((r) => setTimeout(r, 120)) // feel like a network
  const [route, query = ''] = path.split('?')
  const id = Number(route.split('/')[2])

  if (route === '/transactions' && method === 'GET' && query.startsWith('q=')) {
    const q = decodeURIComponent(query.slice(2)).toLowerCase()
    return allTx().filter((t) => [t.merchant, t.note, t.category, t.account].some((v) => v.toLowerCase().includes(q))).sort(byNewest).slice(0, 200)
  }
  if (route === '/transactions' && method === 'POST') {
    const t = { id: nextId++, to_account: '', note: '', account: '', source: 'manual', source_ref: null, confidence: null, needs_review: false, currency: 'IDR', orig_amount: null, ...body }
    if (t.currency !== 'IDR' && !(t.amount > 0)) t.amount = Math.round(t.orig_amount * 16000) // ponytail: rough demo rate
    return put(t)
  }
  if (route.startsWith('/transactions/')) {
    const [list, i] = find(id)
    if (!list) throw new Error('not found')
    const [old] = list.splice(i, 1)
    return method === 'DELETE' ? null : put({ ...old, ...body })
  }

  if (route === '/chat' && method === 'GET') return { enabled: true, sessions: [chatSession], models: [] }
  if (route === '/chat/1' && method === 'GET') return { messages: chatMessages }
  if (route === '/chat' && method === 'POST') {
    const at = new Date().toISOString().replace('T', ' ').slice(0, 19)
    return { session: { id: 2, title: body.text.slice(0, 40), updated_at: at }, messages: [
      { id: Date.now(), role: 'user', text: body.text, created_at: at },
      { id: Date.now() + 1, role: 'assistant', text: reply, model: 'demo', created_at: at },
    ] }
  }
  if (route === '/insight') return { month: body.month, months: body.months, text: reply }

  if (method !== 'GET') return null // other writes are accepted and forgotten
  if (path in routes) return routes[path]
  // months outside the snapshot: reuse the oldest (empty) month's shape
  const m = query.match(/month=(\d{4}-\d{2})/)?.[1]
  if (m) return JSON.parse(JSON.stringify(routes[`${route}?month=${oldest}`]).replaceAll(oldest, m))
  throw new Error(`demo: ${path} tidak tersedia`)
}
