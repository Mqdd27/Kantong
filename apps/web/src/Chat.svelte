<script>
  import { tick } from 'svelte'
  import { api } from './lib/api.js'
  import { act, fmtTime } from './lib/store.svelte.js'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'

  let messages = $state([])
  let models = $state([])
  let model = $state('')
  try { model = localStorage.chatModel || '' } catch {}
  let text = $state('')
  let busy = $state(false)
  let error = $state('')
  let end

  const scroll = () => tick().then(() => end?.scrollIntoView({ block: 'end' }))
  api('/chat')
    .then((r) => {
      messages = r.messages ?? []
      models = r.models ?? []
      if (!models.includes(model)) model = models[0] ?? ''
      scroll()
    })
    .catch((e) => (error = e.message))

  async function send(e) {
    e.preventDefault()
    const q = text.trim()
    if (!q || busy) return
    busy = true
    error = ''
    messages.push({ id: 'pending', role: 'user', text: q })
    text = ''
    scroll()
    try {
      const r = await api('/chat', { method: 'POST', body: { text: q, model } })
      messages.splice(-1, 1, ...r.messages)
    } catch (err) {
      messages.pop()
      text = q
      error = err.message
    }
    busy = false
    scroll()
  }

  async function clear() {
    if (await act('/chat', 'DELETE')) messages = []
  }
  function pick() {
    try { localStorage.chatModel = model } catch {}
  }
  const short = (m) => m.slice(m.indexOf(':') + 1)

  // replies use light markdown: keep the text, turn **bold** into <b>
  const esc = (s) => s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c])
  const rich = (s) => esc(s).replace(/\*\*(.+?)\*\*/g, '<b>$1</b>')
  const keydown = (e) => e.key === 'Enter' && !e.shiftKey && !e.isComposing && send(e)
</script>

<PageHead title="Asisten" />

<p class="muted intro">Dijawab oleh Hermes. Memorinya sama dengan Hermes di Telegram.</p>

<section class="log" aria-live="polite">
  {#each messages as m (m.id)}
    <div class="msg {m.role}">
      <p>{@html rich(m.text)}</p>
      {#if m.created_at}<time>{fmtTime.format(new Date(m.created_at))}{#if m.model} · {short(m.model)}{/if}</time>{/if}
    </div>
  {:else}
    <div class="hello card">
      <span class="avatar" style:--hue="var(--h-teal)"><Icon name="chat" /></span>
      <p>Tanya apa saja soal keuanganmu. Contoh:</p>
      {#each ['Bulan ini paling boros di mana?', 'Sisa budget makan masih aman?', 'Berapa total asetku sekarang?'] as q}
        <button onclick={() => (text = q)}>{q}</button>
      {/each}
    </div>
  {/each}
  {#if busy}<div class="msg assistant typing"><p>Hermes sedang melihat datamu…</p></div>{/if}
  {#if error}<p class="err" role="alert">{error}</p>{/if}
  <div bind:this={end}></div>
</section>

<form class="ask" onsubmit={send}>
  {#if models.length > 1}
    <select bind:value={model} onchange={pick} aria-label="Model">
      {#each models as m}<option value={m}>{short(m)}</option>{/each}
    </select>
  {/if}
  <textarea bind:value={text} onkeydown={keydown} rows="1" placeholder="Tulis pertanyaan…" aria-label="Pertanyaan" maxlength="4000"></textarea>
  <button class="primary" disabled={busy || !text.trim()} aria-label="Kirim"><Icon name="send" /></button>
</form>
{#if messages.length}<button class="clear" onclick={clear}>Hapus riwayat chat</button>{/if}

<style>
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .intro {
    margin: -4px 0 12px;
  }
  .clear {
    border: 0;
    background: none;
    color: var(--muted);
    font-size: 13px;
    padding: 4px 0;
  }
  .clear {
    display: block;
    margin: 8px auto 0;
  }
  .log {
    display: grid;
    gap: 10px;
    padding-bottom: 12px;
  }
  .msg {
    max-width: 85%;
    display: grid;
    gap: 2px;
  }
  .msg p {
    margin: 0;
    padding: 10px 14px;
    border-radius: 16px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 15px;
    line-height: 1.45;
  }
  .user {
    justify-self: end;
    justify-items: end;
  }
  .user p {
    background: var(--accent);
    color: var(--on-accent);
    border-bottom-right-radius: 4px;
  }
  .assistant p {
    background: var(--surface);
    box-shadow: var(--shadow);
    border-bottom-left-radius: 4px;
  }
  .typing p {
    color: var(--muted);
  }
  time {
    color: var(--muted);
    font-size: 11px;
    padding: 0 6px;
  }
  .hello {
    display: grid;
    justify-items: start;
    gap: 8px;
  }
  .hello p {
    margin: 0;
    color: var(--muted);
    font-size: 14px;
  }
  .hello button {
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface);
    color: var(--text);
    padding: 8px 14px;
    font-size: 14px;
    text-align: left;
  }
  .err {
    margin: 0;
    color: var(--out);
    font-size: 14px;
  }
  .ask {
    position: sticky;
    bottom: calc(env(safe-area-inset-bottom) + 84px);
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: end;
    gap: 8px;
    padding: 8px;
    border-radius: 20px;
    background: var(--surface);
    box-shadow: var(--shadow);
  }
  @media (min-width: 900px) {
    .ask {
      bottom: 16px;
    }
  }
  select {
    grid-column: 1 / -1;
    justify-self: start;
    border: 0;
    border-radius: 999px;
    background: var(--bg);
    color: var(--muted);
    padding: 4px 10px;
    font-size: 12px;
  }
  textarea {
    min-height: 40px;
    max-height: 140px;
    field-sizing: content;
    resize: none;
    border: 0;
    background: none;
    padding: 9px 8px;
    font: inherit;
    font-size: 15px;
    color: var(--text);
  }
  textarea:focus {
    outline: none;
  }
  .ask:focus-within {
    outline: 2px solid var(--accent);
  }
  .ask button {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    padding: 0;
    border-radius: 50%;
  }
</style>
