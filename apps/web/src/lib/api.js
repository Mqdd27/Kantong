export class AuthError extends Error {}

export async function api(path, { method = 'GET', body } = {}) {
  // GitHub Pages demo: no server; the mock is dropped from normal builds
  if (import.meta.env.MODE === 'demo') return (await import('../../demo/mock.js')).api(path, { method, body })
  const res = await fetch('/api' + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body && JSON.stringify(body),
  })
  if (res.status === 401) throw new AuthError('unauthorized')
  const data = res.status === 204 ? null : await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.error || `HTTP ${res.status}`)
  return data
}
