<script>
  import { s, fmtDay } from './lib/store.svelte.js'
  import MonthBar from './MonthBar.svelte'
  import TxRow from './TxRow.svelte'

  let { open } = $props()

  const days = $derived.by(() => {
    const groups = []
    for (const t of s.txs) {
      const day = t.occurred_at.slice(0, 10)
      if (groups.at(-1)?.day !== day) groups.push({ day, txs: [] })
      groups.at(-1).txs.push(t)
    }
    return groups
  })
</script>

<MonthBar />

{#each days as g (g.day)}
  <h3>{fmtDay.format(new Date(g.day + 'T12:00:00+07:00'))}</h3>
  {#each g.txs as tx (tx.id)}
    <TxRow {tx} onclick={() => open(tx)} />
  {/each}
{:else}
  <p class="empty">Belum ada transaksi bulan ini.</p>
{/each}

<style>
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
