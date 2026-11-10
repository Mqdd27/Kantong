<script>
  import { s, fmtIDR } from './lib/store.svelte.js'
  import MonthBar from './MonthBar.svelte'
  import TxRow from './TxRow.svelte'
  import BudgetCard from './BudgetCard.svelte'

  let { open } = $props()

  const total = (type) => s.txs.reduce((n, t) => (t.type === type ? n + t.amount : n), 0)
  const inSum = $derived(total('in'))
  const outSum = $derived(total('out'))
  const byCat = $derived.by(() => {
    const m = new Map()
    for (const t of s.txs) if (t.type === 'out') m.set(t.category, (m.get(t.category) || 0) + t.amount)
    return [...m].sort((a, b) => b[1] - a[1])
  })
  const max = $derived(byCat[0]?.[1] || 1)
</script>

<MonthBar />

<div class="cols">
<div>
<section class="totals">
  <div><span>Masuk</span><b class="in">{fmtIDR.format(inSum)}</b></div>
  <div><span>Keluar</span><b>{fmtIDR.format(outSum)}</b></div>
  <div><span>Selisih</span><b class:in={inSum >= outSum}>{fmtIDR.format(inSum - outSum)}</b></div>
</section>

<div class="gap"><BudgetCard compact /></div>

{#if byCat.length}
  <h3>Pengeluaran per kategori</h3>
  <ul class="bars">
    {#each byCat as [cat, amt]}
      <li>
        <span class="label">{cat}</span>
        <span class="bar"><span class="grow" style:width="{(amt / max) * 100}%"></span></span>
        <span class="val">{fmtIDR.format(amt)}</span>
      </li>
    {/each}
  </ul>
{/if}

</div>
<div>
<h3>Terbaru</h3>
{#each s.txs.slice(0, 10) as tx (tx.id)}
  <TxRow {tx} showDate onclick={() => open(tx)} />
{:else}
  <p class="empty">Belum ada transaksi bulan ini.</p>
{/each}
</div>
</div>

<style>
  .gap {
    margin-top: 8px;
  }
  @media (min-width: 900px) {
    .cols {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 40px;
      align-items: start;
    }
    .cols > div > h3:first-child {
      margin-top: 0;
    }
    .totals b {
      font-size: 18px;
    }
  }
  .totals {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
  }
  .totals div {
    background: var(--surface);
    border-radius: 12px;
    padding: 12px;
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .totals span {
    color: var(--muted);
    font-size: 12px;
  }
  .totals b {
    font-size: 15px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  @media (max-width: 459px) {
    .totals {
      grid-template-columns: 1fr 1fr;
    }
    .totals div:last-child {
      grid-column: span 2;
    }
  }
  .in {
    color: var(--in);
  }
  h3 {
    font-size: 14px;
    color: var(--muted);
    margin: 24px 0 8px;
    font-weight: 600;
  }
  .bars {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 8px;
  }
  .bars li {
    display: grid;
    grid-template-columns: 84px 1fr auto;
    gap: 8px;
    align-items: center;
    font-size: 13px;
  }
  .bar {
    height: 8px;
    background: var(--surface);
    border-radius: 4px;
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--accent);
    border-radius: 4px;
  }
  .val {
    font-variant-numeric: tabular-nums;
  }
  .empty {
    color: var(--muted);
  }
</style>
