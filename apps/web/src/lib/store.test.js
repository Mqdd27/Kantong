import { beforeEach, describe, expect, test, vi } from 'vitest'

// store.svelte.js reads localStorage at import time
vi.stubGlobal('localStorage', { hide: '' })
vi.mock('./api.js', async (orig) => ({ ...(await orig()), api: vi.fn() }))
const { api, AuthError } = await import('./api.js')
const st = await import('./store.svelte.js')
const { s, view, mask, fmtIDR, shiftMonth, jakartaLocal, fmtMonth, freqLabel, updateTx, act, addRule, editRule } = st

beforeEach(() => {
  vi.useFakeTimers()
  api.mockReset()
  s.error = ''
  s.authed = true
  view.hide = false
})

describe('shiftMonth', () => {
  test.each([
    ['2026-11', 1, '2026-12'],
    ['2026-12', 1, '2027-01'],
    ['2026-01', -1, '2025-12'],
    ['2026-05', 0, '2026-05'],
    ['2026-03', -15, '2024-12'],
    ['2026-03', 24, '2028-03'],
  ])('%s %+d = %s', (m, d, want) => expect(shiftMonth(m, d)).toBe(want))
})

describe('Jakarta time', () => {
  test('jakartaLocal is UTC+7', () => expect(jakartaLocal(new Date('2026-11-13T03:04:00Z'))).toBe('2026-11-13T10:04'))
  test('jakartaLocal crosses the day and month near UTC midnight', () => {
    expect(jakartaLocal(new Date('2026-01-31T17:30:00Z'))).toBe('2026-02-01T00:30')
    expect(jakartaLocal(new Date('2026-12-31T16:59:00Z'))).toBe('2026-12-31T23:59')
    expect(jakartaLocal(new Date('2026-12-31T17:00:00Z'))).toBe('2027-01-01T00:00')
  })
  test('fmtMonth', () => {
    expect(fmtMonth('2026-11')).toBe('November 2026')
    expect(fmtMonth('2027-01')).toBe('Januari 2027')
  })
})

describe('freqLabel', () => {
  test.each([
    [{ freq: 'weekly', mask: 62 }, 'hari kerja'],
    [{ freq: 'weekly', mask: 0b1000001 }, 'tiap Min, Sab'],
    [{ freq: 'weekly', mask: 0b10 }, 'tiap Sen'],
    [{ freq: 'weekly', mask: 0 }, 'mingguan'],
    [{ freq: 'monthly', mask: 0, next_date: '2026-11-05' }, 'bulanan'],
    [{ freq: 'monthly', mask: (1 << 0) | (1 << 6), next_date: '2027-01-05' }, 'tgl 5 bulan Jan, Jul'],
    [{ freq: 'yearly', mask: 0, next_date: '2027-03-01' }, 'tahunan'],
  ])('%o = %s', (r, want) => expect(freqLabel(r)).toBe(want))
})

describe('mask', () => {
  test('shown', () => {
    expect(mask('Rp 5')).toBe('Rp 5')
    expect(fmtIDR.format(1250000).replace(/\s/g, ' ')).toBe('Rp 1.250.000')
  })
  test('hidden', () => {
    view.hide = true
    expect(mask('Rp 5')).toBe('••••••')
    expect(fmtIDR.format(1250000)).toBe('••••••')
  })
  test('initial value comes from localStorage', async () => {
    vi.resetModules()
    vi.stubGlobal('localStorage', { hide: '1' })
    const fresh = await import('./store.svelte.js')
    expect(fresh.view.hide).toBe(true)
    vi.stubGlobal('localStorage', { hide: '' })
  })
})

describe('updateTx', () => {
  const tx = { id: 7, occurred_at: '', amount: 100, type: 'out', category: 'Makan' }
  beforeEach(() => {
    tx.occurred_at = s.month + '-05T10:00:00+07:00'
    s.txs = [{ ...tx }]
  })

  test('applies the patch, then the server row', async () => {
    let resolve
    api.mockReturnValue(new Promise((r) => (resolve = r)))
    const p = updateTx(7, { amount: 200 })
    expect(s.txs[0].amount).toBe(200) // optimistic
    resolve({ ...tx, amount: 200, category: 'Jajan' })
    await p
    expect(api).toHaveBeenCalledWith('/transactions/7', { method: 'PATCH', body: { amount: 200 } })
    expect(s.txs[0].category).toBe('Jajan')
  })

  test('rolls back and reports on failure', async () => {
    api.mockRejectedValue(new Error('nope'))
    await updateTx(7, { amount: 200 })
    expect(s.txs[0].amount).toBe(100)
    expect(s.error).toBe('nope')
    vi.advanceTimersByTime(4000)
    expect(s.error).toBe('')
  })

  test('unknown id does nothing', async () => {
    await updateTx(99, { amount: 1 })
    expect(api).not.toHaveBeenCalled()
  })
})

describe('act', () => {
  test('returns true on success', async () => {
    api.mockResolvedValue(null)
    expect(await act('/budget', 'PUT', { a: 1 })).toBe(true)
    expect(api).toHaveBeenCalledWith('/budget', { method: 'PUT', body: { a: 1 } })
  })

  test('returns undefined and reports the error', async () => {
    api.mockRejectedValue(new Error('boom'))
    expect(await act('/x', 'POST')).toBeUndefined()
    expect(s.error).toBe('boom')
  })

  test('AuthError logs out instead of showing an error', async () => {
    api.mockRejectedValue(new AuthError('unauthorized'))
    expect(await act('/x', 'POST')).toBeUndefined()
    expect(s.authed).toBe(false)
    expect(s.error).toBe('')
  })
})

describe('rules', () => {
  test('addRule trims the merchant and POSTs', async () => {
    api.mockResolvedValue({})
    await addRule('  Indomaret ', 'out', 'Belanja')
    expect(api).toHaveBeenCalledWith('/rules', { method: 'POST', body: { pattern: 'Indomaret', type: 'out', category: 'Belanja' } })
  })

  test('editRule PATCHes the rule', async () => {
    api.mockResolvedValue({})
    await editRule(3, { category: 'Jajan' })
    expect(api).toHaveBeenCalledWith('/rules/3', { method: 'PATCH', body: { category: 'Jajan' } })
  })
})
