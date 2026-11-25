<script>
  import { onMount } from 'svelte'
  import { s, fmtIDR, loadAssets } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import ReviewAlert from './ReviewAlert.svelte'
  import TxRow from './TxRow.svelte'
  import BudgetCard from './BudgetCard.svelte'
  import SpendCard from './SpendCard.svelte'
  import Icon from './Icon.svelte'

  let { open } = $props()

  onMount(() => {
    if (!s.portfolio.assets.length) loadAssets() // net worth tile
  })

  const total = (type) => s.txs.reduce((n, t) => (t.type === type ? n + t.amount : n), 0)
  const inSum = $derived(total('in'))
  const outSum = $derived(total('out'))
  const byCat = $derived.by(() => {
    const m = new Map()
    for (const t of s.txs) if (t.type === 'out') m.set(t.category, (m.get(t.category) || 0) + t.amount)
    return [...m].sort((a, b) => b[1] - a[1])
  })
  const max = $derived(byCat[0]?.[1] || 1)
  // savings rate as Analitik counts it: Investasi is saving, not spending
  const spend = $derived(outSum - (byCat.find(([c]) => c === 'Investasi')?.[1] ?? 0))
  const rate = $derived(inSum ? Math.round(((inSum - spend) / inSum) * 100) : null)
  // before payday the rate is meaningless (e.g. -300%), so show spending instead
  const paid = $derived(s.txs.some((t) => t.type === 'in' && t.category === 'Gaji'))
</script>

<PageHead title="Beranda">
  {#if s.chat}<a class="icon" href="#/chat" aria-label="Tanya asisten"><Icon name="chat" /></a>{/if}
  <a class="icon" href="#/devices" aria-label="Perangkat & keluar" title="Perangkat & keluar"><Icon name="laptop" /></a>
</PageHead>
<MonthBar />

<div class="home">
  <div class="stack a">
    <SpendCard spent={spend} {byCat} income={inSum} />
    {#if s.budget?.overall}<BudgetCard compact />{/if}

    <ReviewAlert />

    {#if paid}
      <a class="card tile" href="#/analytics">
        <span class="avatar sm" style:--hue="var(--h-teal)"><Icon name="chart" size={18} /></span>
        <span class="lbl">Tabungan bulan ini</span>
        <b class="num">{rate == null ? '–' : rate + '%'}</b>
      </a>
    {/if}
  </div>

  <div class="stack c">
    <div class="sec"><h2>Pengeluaran per kategori</h2><a href="#/analytics">Analitik</a></div>
    <section class="card">
      {#if byCat.length}
        <ul class="bars">
          {#each byCat as [cat, amt]}
            {@const look = catLook(cat)}
            <li><a href="#/tx?cat={encodeURIComponent(cat)}">
              <span class="avatar sm" style:--hue={look.hue}><Icon name={look.icon} size={18} /></span>
              <span class="what">
                <span class="line"><span>{cat}</span><span class="num">{fmtIDR.format(amt)}</span></span>
                <span class="bar"><span class="grow" style:width="{(amt / max) * 100}%" style:background={look.hue}></span></span>
              </span>
            </a></li>
          {/each}
        </ul>
      {:else}
        <p class="empty">Belum ada pengeluaran bulan ini.</p>
      {/if}
    </section>
  </div>

  <div class="stack b">
    <div class="sec"><h2>Transaksi terbaru</h2><a href="#/tx">Lihat semua</a></div>
    <section class="card list">
      {#each s.txs.slice(0, 10) as tx (tx.id)}
        <TxRow {tx} showDate onclick={() => open(tx)} />
      {:else}
        <p class="empty">Belum ada transaksi bulan ini.</p>
      {/each}
    </section>
  </div>
</div>

<style>
  /* phone: balance, recent, categories; desktop: balance + categories left, recent right */
  .home {
    display: grid;
    gap: 12px;
  }
  .home > * {
    min-width: 0;
  }
  .c {
    order: 1; /* phone: categories after recent transactions; desktop uses grid areas */
  }
  @media (min-width: 900px) {
    .home {
      grid-template-columns: 1fr 1fr;
      grid-template-areas: 'a b' 'c b';
      align-items: start;
      gap: 12px 24px;
    }
    .a {
      grid-area: a;
    }
    .b {
      grid-area: b;
    }
    .c {
      grid-area: c;
    }
    .b .sec {
      margin-top: 0;
    }
  }
  .tile {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text);
    text-decoration: none;
    min-width: 0;
  }
  .tile b {
    margin-left: auto;
  }
  .lbl {
    color: var(--muted);
    font-size: 13px;
  }
  .tile b {
    font-size: clamp(15px, 4.4vw, 17px);
  }
  .bars {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 2px;
  }
  .bars a {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 44px;
    margin: 0 -8px;
    padding: 4px 8px;
    border-radius: 10px;
    color: inherit;
    text-decoration: none;
  }
  .bars a:hover,
  .bars a:focus-visible {
    background: var(--bg);
  }
  .what {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 6px;
  }
  .line {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: 14px;
  }
  .line .num {
    font-weight: 600;
  }
  .bar {
    height: 6px;
    background: var(--bg);
    border-radius: 3px;
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: 3px;
  }
</style>
