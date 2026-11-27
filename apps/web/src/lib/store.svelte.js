import { api, AuthError } from './api.js'

const TZ = 'Asia/Jakarta'
const idr = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 })
// eye toggle: every money display goes through mask(); remembered per device
let hidden = false
try { hidden = localStorage.hide === '1' } catch {}
export const view = $state({ hide: hidden })
export function toggleHide() {
  view.hide = !view.hide
  try { localStorage.hide = view.hide ? '1' : '' } catch {}
}
export const mask = (str) => (view.hide ? '••••••' : str)
export const fmtIDR = { format: (n) => mask(idr.format(n)) }
export const fmtNum = new Intl.NumberFormat('id-ID')
export const fmtDay = new Intl.DateTimeFormat('id-ID', { weekday: 'long', day: 'numeric', month: 'long', timeZone: TZ })
export const fmtShort = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: TZ })
export const fmtTime = new Intl.DateTimeFormat('id-ID', { hour: '2-digit', minute: '2-digit', timeZone: TZ })
const fmtMonthName = new Intl.DateTimeFormat('id-ID', { month: 'long', year: 'numeric', timeZone: TZ })

/** "YYYY-MM-DDTHH:mm" wall clock in Jakarta (UTC+7, no DST). */
export const jakartaLocal = (d = new Date()) => new Date(d.getTime() + 7 * 3600e3).toISOString().slice(0, 16)
export const fmtMonth = (m) => fmtMonthName.format(new Date(m + '-15T00:00:00+07:00'))
export function shiftMonth(m, delta) {
  const [y, mo] = m.split('-').map(Number)
  return new Date(Date.UTC(y, mo - 1 + delta, 1)).toISOString().slice(0, 7)
}

export const s = $state({
  month: jakartaLocal().slice(0, 7),
  txs: [],
  categories: [],
  authed: true,
  live: false,
  error: '',
  portfolio: { assets: [], total_idr: 0, pending_orders: [], missing_rates: [] },
  history: [],
  shared: [], // kantong bersama the account belongs to, with members (GET /api/shared)
  budget: null,
  recurring: [],
  chat: false, // AI assistant: on only when the server has HERMES_API_URL
})

const byNewest = (a, b) => (a.occurred_at < b.occurred_at ? 1 : a.occurred_at > b.occurred_at ? -1 : b.id - a.id)

// ids that just arrived (email import, another device, this device): rows mounting in the next moment mark the landing
const fresh = new Set()
export const isFresh = (id) => fresh.has(id)

function upsert(t) {
  const i = s.txs.findIndex((x) => x.id === t.id)
  if (t.occurred_at.slice(0, 7) !== s.month) {
    if (i >= 0) s.txs.splice(i, 1)
    return
  }
  if (i >= 0) s.txs[i] = t
  else {
    s.txs.push(t)
    fresh.add(t.id)
    setTimeout(() => fresh.delete(t.id), 1500)
  }
  s.txs.sort(byNewest)
}

function remove(id) {
  const i = s.txs.findIndex((x) => x.id === id)
  if (i >= 0) s.txs.splice(i, 1)
}

/** Back to the login screen with nothing from the previous session left in memory. */
function signedOut() {
  es?.close()
  clearTimeout(retry)
  Object.assign(s, { authed: false, live: false, txs: [], categories: [], history: [], shared: [], budget: null, recurring: [], chat: false,
    portfolio: { assets: [], total_idr: 0, pending_orders: [], missing_rates: [] } })
}

let errTimer
function fail(e) {
  if (e instanceof AuthError) return signedOut()
  s.error = e.message
  clearTimeout(errTimer)
  errTimer = setTimeout(() => (s.error = ''), 4000)
}

export async function load(force = false) {
  const month = s.month
  try {
    const [txs, cats] = await Promise.all([api(`/transactions?month=${month}`), s.categories.length && !force ? null : api('/categories')])
    if (cats) s.categories = cats
    if (month === s.month) s.txs = txs
    loadBudget()
  } catch (e) {
    fail(e)
  }
}

export async function loadCategories() {
  try {
    s.categories = await api('/categories')
  } catch (e) {
    fail(e)
  }
}

/**
 * Adds an in/out category and reloads the list. Returns `{ category }` on success or `{ error }` with a message
 * for the caller to show next to its input (the global error banner sits behind open sheets).
 */
export async function addCategory(name, type) {
  try {
    const category = await api('/categories', { method: 'POST', body: { name, type } })
    await loadCategories()
    return { category }
  } catch (e) {
    if (e instanceof AuthError) {
      s.authed = false
      return { error: '' }
    }
    return { error: CATEGORY_ERRORS[e.message] ?? e.message }
  }
}
/** Renames a user-created category. Returns `{ category }` or `{ error }` like addCategory. */
export async function renameCategory(id, name) {
  try {
    const category = await api(`/categories/${id}`, { method: 'PATCH', body: { name } })
    // names are copied into transactions, rules, budgets and recurring items: reload everything that shows them
    await Promise.all([loadCategories(), load(true)])
    return { category }
  } catch (e) {
    if (e instanceof AuthError) {
      s.authed = false
      return { error: '' }
    }
    return { error: CATEGORY_ERRORS[e.message] ?? e.message }
  }
}
/** Deletes a user-created category: its transactions move to Lainnya. Returns the API result or `{ error }`. */
export async function deleteCategory(id) {
  try {
    const r = await api(`/categories/${id}`, { method: 'DELETE' })
    await Promise.all([loadCategories(), load(true)])
    return r
  } catch (e) {
    if (e instanceof AuthError) {
      s.authed = false
      return { error: '' }
    }
    return { error: CATEGORY_ERRORS[e.message] ?? e.message }
  }
}
const CATEGORY_ERRORS = {
  'category already exists': 'Kategori itu sudah ada.',
  'name is required': 'Nama kategori wajib diisi.',
  'built-in categories cannot be renamed': 'Kategori bawaan tidak bisa diubah.',
  'built-in categories cannot be deleted': 'Kategori bawaan tidak bisa dihapus.',
}

export function setMonth(m) {
  s.month = m
  s.txs = []
  load()
}

// ---- realtime ----
let es, retry
export function connect() {
  es?.close()
  clearTimeout(retry)
  if (!s.authed) return
  if (import.meta.env.MODE === 'demo') return void (s.live = true)
  es = new EventSource('/api/events')
  es.onopen = () => (s.live = true)
  es.addEventListener('transaction.created', (e) => upsert(JSON.parse(e.data)))
  es.addEventListener('transaction.updated', (e) => upsert(JSON.parse(e.data)))
  es.addEventListener('transaction.deleted', (e) => remove(JSON.parse(e.data).id))
  es.addEventListener('assets.changed', loadAssets)
  es.addEventListener('budget.changed', budgetSoon)
  es.addEventListener('categories.changed', loadCategories)
  es.addEventListener('transactions.changed', () => load()) // a kantong bersama and its transactions were deleted
  // asset balances follow transactions on their account, so refresh them too once loaded
  const txChanged = () => { budgetSoon(); if (s.portfolio.assets.length) loadAssets() }
  for (const ev of ['transaction.created', 'transaction.updated', 'transaction.deleted']) es.addEventListener(ev, txChanged)
  es.onerror = () => {
    // iOS drops the stream when the app is backgrounded: reconnect and refetch.
    s.live = false
    es.close()
    retry = setTimeout(() => {
      load()
      connect()
    }, 3000)
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible' && s.authed) {
      load()
      loadAssets()
      connect()
    }
  })
}

// ---- optimistic writes ----
export async function createTx(t) {
  const temp = { ...t, id: -Date.now(), source: 'manual', source_ref: null, confidence: null, needs_review: false }
  upsert(temp)
  try {
    const saved = await api('/transactions', { method: 'POST', body: t })
    remove(temp.id)
    upsert(saved) // server fills the IDR amount for foreign-currency spends
  } catch (e) {
    remove(temp.id)
    fail(e)
  }
}

export async function updateTx(id, patch) {
  const old = s.txs.find((x) => x.id === id)
  if (!old) return
  const before = $state.snapshot(old)
  upsert({ ...before, ...patch })
  try {
    upsert(await api(`/transactions/${id}`, { method: 'PATCH', body: patch }))
  } catch (e) {
    upsert(before)
    fail(e)
  }
}

export async function deleteTx(id) {
  const old = s.txs.find((x) => x.id === id)
  const before = old && $state.snapshot(old)
  remove(id)
  try {
    await api(`/transactions/${id}`, { method: 'DELETE' })
  } catch (e) {
    if (before) upsert(before)
    fail(e)
  }
}

export async function login(username, password) {
  await api('/login', { method: 'POST', body: { username, password } })
  s.authed = true
}

export async function logout() {
  try {
    await api('/logout', { method: 'POST' })
    signedOut()
  } catch (e) {
    fail(e)
  }
}

// ---- account ----
export const loadMe = () => api('/me').catch((e) => (fail(e), null))
/** Connects (or with empty values disconnects) this account's Telegram bot. Returns the new /me, or throws the server's message. */
/** Sets a new password; other devices are signed out. Throws the server's message. */
export const changePassword = (current, next) => api('/me/password', { method: 'PUT', body: { current, new: next } })
export const saveTelegram = (bot_token, chat_id) => api('/me/telegram', { method: 'PUT', body: { bot_token, chat_id } })

// ---- devices (login sessions) ----
export const loadSessions = () => api('/sessions').catch((e) => (fail(e), []))
/** Signs one device out; signing out this device returns to the login screen. Returns true on success. */
export async function endSession(sess) {
  try {
    await api(`/sessions/${encodeURIComponent(sess.id)}`, { method: 'DELETE' })
    if (sess.current) signedOut()
    return true
  } catch (e) {
    fail(e)
  }
}
export const revokeOthers = () => act('/sessions/revoke', 'POST')

/** "iPhone · Safari" from a User-Agent. iPadOS Safari claims to be a Mac, so it shows as Mac. */
export function deviceName(ua = '') {
  const os = [['iPhone', 'iPhone'], ['iPad', 'iPad'], ['Android', 'Android'], ['Macintosh', 'Mac'], ['Windows', 'Windows'], ['Linux', 'Linux']].find(([k]) => ua.includes(k))?.[1]
  const app = [['Edg', 'Edge'], ['Firefox', 'Firefox'], ['FxiOS', 'Firefox'], ['CriOS', 'Chrome'], ['Chrome', 'Chrome'], ['Safari', 'Safari']].find(([k]) => ua.includes(k))?.[1]
  // iOS home-screen apps drop "Safari" from the UA
  return [os ?? 'Perangkat lain', app ?? (os === 'iPhone' || os === 'iPad' ? 'Aplikasi' : '')].filter(Boolean).join(' · ')
}
export const deviceIcon = (ua = '') => (/iPhone|Android.*Mobile/.test(ua) ? 'phone' : /iPad|Android/.test(ua) ? 'tablet' : 'laptop')

// ---- assets ----
export const CURRENCIES = ['IDR', 'USD', 'SGD', 'CNY', 'JPY', 'KRW', 'EUR', 'AUD', 'GBP', 'HKD', 'MYR', 'THB', 'XAU']
// XAU balances are grams of gold (priced per gram by the API)
export const fmtCur = (amount, currency = 'IDR') => mask(
  currency === 'XAU' ? `${new Intl.NumberFormat('id-ID', { maximumFractionDigits: 4 }).format(amount)} gram` :
  new Intl.NumberFormat('id-ID', { style: 'currency', currency, maximumFractionDigits: currency === 'IDR' ? 0 : 2 }).format(amount))

export async function loadAssets() {
  try {
    s.portfolio = await api('/assets') // also records today's snapshot
    s.history = await api('/assets/history')
    s.shared = await api('/shared')
  } catch (e) {
    fail(e)
  }
}

// ponytail: not optimistic; the SSE assets.changed event refreshes the list.
export async function saveAsset(id, a) {
  try {
    await api(id ? `/assets/${id}` : '/assets', { method: id ? 'PATCH' : 'POST', body: a })
    return true
  } catch (e) {
    fail(e)
  }
}

export async function deleteAsset(id) {
  try {
    await api(`/assets/${id}`, { method: 'DELETE' })
  } catch (e) {
    fail(e)
  }
}

// ---- budget & recurring ----
let budgetTimer
const budgetSoon = () => {
  clearTimeout(budgetTimer)
  budgetTimer = setTimeout(loadBudget, 300) // coalesce bursts of SSE events
}

export async function loadBudget() {
  const month = s.month
  try {
    const [b, rec] = await Promise.all([api(`/budget?month=${month}`), api('/recurring')])
    if (month === s.month) s.budget = b
    s.recurring = rec
  } catch (e) {
    fail(e)
  }
}

/** Runs an API write; the SSE event then refreshes the view. Returns true on success. */
export async function act(path, method, body) {
  try {
    await api(path, { method, body })
    budgetSoon()
    return true
  } catch (e) {
    fail(e)
  }
}

export const setBudget = (category, amount) => act('/budget', 'PUT', { month: s.month, category, amount })

// ---- rules ----
export const addRule = (merchant, type, category) => act('/rules', 'POST', { pattern: merchant.trim(), type, category })
export const editRule = (id, patch) => act(`/rules/${id}`, 'PATCH', patch)

// recurring mask: weekly bit 0 = Minggu … 6 = Sabtu; monthly bit 0 = Jan … 11 = Des
export const DAYS = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab']
export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des']
export function freqLabel(r) {
  const pick = (names) => names.filter((_, i) => r.mask & (1 << i)).join(', ')
  if (r.freq === 'weekly') return r.mask === 62 ? 'hari kerja' : r.mask ? 'tiap ' + pick(DAYS) : 'mingguan'
  if (r.freq === 'monthly') return r.mask ? 'tgl ' + Number(r.next_date.slice(8)) + ' bulan ' + pick(MONTHS) : 'bulanan'
  return 'tahunan'
}
