import { afterEach, describe, expect, test, vi } from 'vitest'
import { api, AuthError } from './api.js'

const respond = (status, body) => vi.stubGlobal('fetch', vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status })))
afterEach(() => vi.unstubAllGlobals())

describe('api', () => {
  test('GET without body sends no content type', async () => {
    respond(200, [1])
    expect(await api('/x')).toEqual([1])
    expect(fetch).toHaveBeenCalledWith('/api/x', { method: 'GET', headers: {}, body: undefined })
  })

  test('body is sent as JSON', async () => {
    respond(201, { id: 1 })
    await api('/transactions', { method: 'POST', body: { amount: 5 } })
    expect(fetch).toHaveBeenCalledWith('/api/transactions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"amount":5}' })
  })

  test('401 throws AuthError', async () => {
    respond(401, { error: 'unauthorized' })
    await expect(api('/x')).rejects.toBeInstanceOf(AuthError)
  })

  test('error uses the server message', async () => {
    respond(400, { error: 'amount must be positive' })
    await expect(api('/x')).rejects.toThrow('amount must be positive')
  })

  test('error without JSON falls back to HTTP status', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>bad gateway', { status: 502 })))
    const err = await api('/x').catch((e) => e)
    expect(err.message).toBe('HTTP 502')
    expect(err).not.toBeInstanceOf(AuthError)
  })

  test('204 returns null', async () => {
    respond(204)
    expect(await api('/x', { method: 'DELETE' })).toBeNull()
  })
})
