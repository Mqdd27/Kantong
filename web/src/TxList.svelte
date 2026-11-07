<script>
  import { s, fmtDay, fmtIDR } from './lib/store.svelte.js'
  import { api } from './lib/api.js'
  import MonthBar from './MonthBar.svelte'
  import TxRow from './TxRow.svelte'

  let { open } = $props()

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
  const list = $derived(found ?? s.txs)
  const total = $derived(list.reduce((n, t) => n + (t.type === 'out' ? -t.amount : t.type === 'in' ? t.amount : 0), 0))

  const days = $derived.by(() => {
    const groups = []
    for (const t of list) {
      const day = t.occurred_at.slice(0, 10)
      if (groups.at(-1)?.day !== day) groups.push({ day, txs: [] })
      groups.at(-1).txs.push(t)
    }
    return groups
  })
</script>

<div class="tools">
  <input type="search" bind:value={q} placeholder="Cari merchant, catatan, kategori…" aria-label="Cari transaksi" />
  <a class="export" href="#/rules">Aturan</a>
  <a class="export" href="/api/export.csv" download>Export CSV</a>
</div>

{#if found}
  <p class="sum">{found.length}{found.length === 200 ? '+' : ''} hasil di semua bulan · bersih {fmtIDR.format(total)}</p>
{:else}
  <MonthBar />
{/if}

{#each days as g (g.day)}
  <h3>{fmtDay.format(new Date(g.day + 'T12:00:00+07:00'))}{found ? ' ' + g.day.slice(0, 4) : ''}</h3>
  {#each g.txs as tx (tx.id)}
    <TxRow {tx} onclick={() => open(tx)} />
  {/each}
{:else}
  <p class="empty">{found ? 'Nggak ketemu.' : 'Belum ada transaksi bulan ini.'}</p>
{/each}

<style>
  .tools {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
  }
  .export {
    flex: none;
    align-self: center;
    color: var(--muted);
    font-size: 13px;
    padding: 8px 4px;
  }
  .sum {
    color: var(--muted);
    font-size: 13px;
    margin: 0 0 8px;
  }
  h3 {
    font-size: 13px;
    color: var(--muted);
    margin: 20px 0 0;
    font-weight: 600;
  }
  .empty {
    color: var(--muted);
  }
</style>
