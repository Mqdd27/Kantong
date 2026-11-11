<script>
  import { onMount } from 'svelte'
  import { s, fmtIDR, loadAssets } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import ReviewAlert from './ReviewAlert.svelte'
  import TxRow from './TxRow.svelte'
  import BudgetCard from './BudgetCard.svelte'
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
</PageHead>
<MonthBar />

<div class="home">
  <div class="stack a">
    <section class="balance">
      <BudgetCard compact hero />
      <div class="flow">
        <div><span>Masuk</span><b class="num">{fmtIDR.format(inSum)}</b></div>
        <div><span>Keluar</span><b class="num">{fmtIDR.format(outSum)}</b></div>
        <div><span>Selisih</span><b class="num">{fmtIDR.format(inSum - outSum)}</b></div>
      </div>
    </section>

    <ReviewAlert />

    <div class="tiles">
      <a class="card tile" href="#/assets">
        <span class="avatar sm" style:--hue="var(--h-blue)"><Icon name="wallet" size={18} /></span>
        <span class="lbl">Total aset</span>
        <b class="num">{fmtIDR.format(s.portfolio.total_idr)}</b>
      </a>
      <a class="card tile" href="#/analytics">
        <span class="avatar sm" style:--hue="var(--h-teal)"><Icon name="chart" size={18} /></span>
        {#if paid}
          <span class="lbl">Tabungan bulan ini</span>
          <b class="num">{rate == null ? '–' : rate + '%'}</b>
        {:else}
          <span class="lbl">Keluar bulan ini</span>
          <b class="num">{fmtIDR.format(spend)}</b>
        {/if}
      </a>
    </div>
  </div>

  <div class="stack c">
    <div class="sec"><h2>Pengeluaran per kategori</h2><a href="#/analytics">Analitik</a></div>
    <section class="card">
      {#if byCat.length}
        <ul class="bars">
          {#each byCat as [cat, amt]}
            {@const look = catLook(cat)}
            <li>
              <span class="avatar sm" style:--hue={look.hue}><Icon name={look.icon} size={18} /></span>
              <span class="what">
                <span class="line"><span>{cat}</span><span class="num">{fmtIDR.format(amt)}</span></span>
                <span class="bar"><span class="grow" style:width="{(amt / max) * 100}%" style:background={look.hue}></span></span>
              </span>
            </li>
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
  .balance {
    display: grid;
    gap: 16px;
    padding: 18px 18px 16px;
    border-radius: 20px;
    background: var(--accent);
    color: var(--hero-text);
    box-shadow: 0 8px 24px color-mix(in srgb, var(--accent) 28%, transparent);
  }
  .flow {
    display: grid;
    grid-template-columns: repeat(3, auto);
    justify-content: space-between;
    gap: 8px;
    padding-top: 14px;
    border-top: 1px solid var(--hero-track);
  }
  .flow div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .flow span {
    color: var(--hero-muted);
    font-size: 12px;
  }
  .flow b {
    font-size: clamp(12px, 3.6vw, 14px);
  }
  .tiles {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .tile {
    display: grid;
    gap: 4px;
    color: var(--text);
    text-decoration: none;
    min-width: 0;
  }
  .tile .avatar {
    margin-bottom: 6px;
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
    gap: 14px;
  }
  .bars li {
    display: flex;
    align-items: center;
    gap: 12px;
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
