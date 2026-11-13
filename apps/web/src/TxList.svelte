<script>
  import { s, fmtDay, fmtIDR } from './lib/store.svelte.js'
  import { api } from './lib/api.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import ReviewAlert from './ReviewAlert.svelte'
  import TxRow from './TxRow.svelte'
  import ViewToggle from './ViewToggle.svelte'
  import Icon from './Icon.svelte'

  let { open, cat } = $props()

  // one chip per name (Lainnya / Penyesuaian exist as both in and out), spending first
  const ORDER = { out: 0, in: 1, transfer: 2 }
  const chips = $derived([...new Set(s.categories.toSorted((a, b) => (ORDER[a.type] ?? 3) - (ORDER[b.type] ?? 3)).map((c) => c.name))])
  const active = $derived(chips.includes(cat) ? cat : null) // unknown names in the URL are ignored
  const pick = (name) => (location.hash = name && name !== active ? '#/tx?cat=' + encodeURIComponent(name) : '#/tx')

  // chip row scrolls sideways; arrows page through it and dim at either end
  let row = $state()
  let canL = $state(false)
  let canR = $state(false)
  const edges = () => {
    if (!row) return
    canL = row.scrollLeft > 1
    canR = row.scrollLeft + row.clientWidth < row.scrollWidth - 1
  }
  const slide = (dir) => row.scrollBy({ left: dir * row.clientWidth * 0.8, behavior: 'smooth' })
  $effect(() => {
    chips.length
    row?.querySelector('[aria-pressed="true"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    edges()
  })

  let q = $state('')
  let found = $state(null) // search results across all months, null = not searching
  let timer
  $effect(() => {
    const term = q.trim()
    clearTimeout(timer)
    if (!term) return void (found = null)
    timer = setTimeout(() => api('/transactions?q=' + encodeURIComponent(term)).then((r) => q.trim() === term && (found = r)).catch(() => {}), 250)
  })
  $effect(() => {
    s.txs // re-run the search when transactions change
    if (found && q.trim()) api('/transactions?q=' + encodeURIComponent(q.trim())).then((r) => (found = r)).catch(() => {})
  })
  const list = $derived((found ?? s.txs).filter((t) => !active || t.category === active))
  const net = (txs) => txs.reduce((n, t) => n + (t.type === 'out' ? -t.amount : t.type === 'in' ? t.amount : 0), 0)
  const sum = (type) => list.reduce((n, t) => (t.type === type ? n + t.amount : n), 0)
  const inSum = $derived(sum('in'))
  const outSum = $derived(sum('out'))

  const days = $derived.by(() => {
    const groups = []
    for (const t of list) {
      const day = t.occurred_at.slice(0, 10)
      if (groups.at(-1)?.day !== day) groups.push({ day, txs: [] })
      groups.at(-1).txs.push(t)
    }
    return groups
  })
  const signed = (n) => (n > 0 ? '+' : n < 0 ? '−' : '') + fmtIDR.format(Math.abs(n))
</script>

<svelte:window onresize={edges} />

<PageHead title="Transaksi">
  <a class="icon" href="#/rules" aria-label="Aturan kategori" title="Aturan kategori"><Icon name="tag" /></a>
  <a class="icon" href="api/export.xlsx" download aria-label="Export Excel" title="Export Excel"><Icon name="download" /></a>
</PageHead>

<ViewToggle active="tx" />

<label class="search">
  <Icon name="search" size={18} />
  <input type="search" bind:value={q} placeholder="Cari merchant, catatan, kategori…" aria-label="Cari transaksi" />
</label>

{#if chips.length}
  <div class="chipbar">
    <button class="nav" aria-label="Geser kategori ke kiri" disabled={!canL} onclick={() => slide(-1)}><Icon name="left" size={18} /></button>
    <div class="chips" role="group" aria-label="Filter kategori" bind:this={row} onscroll={edges}>
      <button aria-pressed={!active} onclick={() => pick(null)}>Semua</button>
      {#each chips as name}
        <button aria-pressed={active === name} onclick={() => pick(name)}>{name}</button>
      {/each}
    </div>
    <button class="nav" aria-label="Geser kategori ke kanan" disabled={!canR} onclick={() => slide(1)}><Icon name="right" size={18} /></button>
  </div>
{/if}

{#if !found}<MonthBar />{/if}

<div class="stack">
  {#if !found}<ReviewAlert />{/if}

  <section class="card summary">
    <div><span>Masuk</span><b class="num pos">{fmtIDR.format(inSum)}</b></div>
    <div><span>Keluar</span><b class="num">{fmtIDR.format(outSum)}</b></div>
    <div><span>{found ? 'Bersih' : 'Selisih'}</span><b class="num">{signed(inSum - outSum)}</b></div>
  </section>
  {#if found}
    <p class="hits">{found.length}{found.length === 200 ? '+' : ''} hasil di semua bulan</p>
  {/if}

  {#each days as g (g.day)}
    <section class="card list">
      <h3>
        <span>{fmtDay.format(new Date(g.day + 'T12:00:00+07:00'))}{found ? ' ' + g.day.slice(0, 4) : ''}</span>
        <span class="num">{signed(net(g.txs))}</span>
      </h3>
      {#each g.txs as tx (tx.id)}
        <TxRow {tx} onclick={() => open(tx)} />
      {/each}
    </section>
  {:else}
    <section class="card">
      {#if active}
        <p class="empty">Tidak ada transaksi {active} {found ? 'yang cocok' : 'bulan ini'}.</p>
        <p class="empty"><button onclick={() => pick(null)}>Hapus filter</button></p>
      {:else}
        <p class="empty">{found ? 'Tidak ketemu.' : 'Belum ada transaksi bulan ini.'}</p>
      {/if}
    </section>
  {/each}
</div>

<style>
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
    padding: 0 14px;
    border-radius: 999px;
    background: var(--surface);
    box-shadow: var(--shadow);
    color: var(--muted);
  }
  .search input {
    border: 0;
    padding: 11px 0;
    background: none;
    outline: none;
  }
  .search:focus-within {
    outline: 2px solid var(--accent);
  }
  .chipbar {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 12px;
  }
  .chips {
    flex: 1;
    display: flex;
    gap: 6px;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .chips::-webkit-scrollbar {
    display: none;
  }
  .nav {
    flex: none;
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    padding: 0;
    border-radius: 50%;
  }
  .nav:disabled {
    opacity: 0.35;
  }
  .chips button {
    flex: none;
    padding: 7px 14px;
    border-radius: 999px;
    font-size: 14px;
    white-space: nowrap;
  }
  .chips [aria-pressed='true'] {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, var(--surface));
    color: var(--accent);
    font-weight: 600;
  }
  .summary {
    display: grid;
    grid-template-columns: repeat(3, auto);
    justify-content: space-between;
    gap: 8px;
  }
  .summary div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .summary span {
    color: var(--muted);
    font-size: 12px;
  }
  .summary b {
    font-size: clamp(12px, 3.6vw, 14px);
  }
  .hits {
    margin: -4px 0 0;
    color: var(--muted);
    font-size: 13px;
  }
  h3 {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin: 0;
    padding: 12px 0 8px;
    border-bottom: 1px solid var(--line);
    font-size: 13px;
    font-weight: 600;
    color: var(--muted);
  }
  h3 .num {
    color: var(--text);
  }
</style>
