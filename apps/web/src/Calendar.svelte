<script>
  import { s, fmtIDR, fmtDay, mask } from './lib/store.svelte.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import ViewToggle from './ViewToggle.svelte'
  import TxRow from './TxRow.svelte'

  let { open } = $props()

  const shortRaw = new Intl.NumberFormat('id-ID', { notation: 'compact', maximumSignificantDigits: 2 }) // fits a day cell
  const short = { format: (n) => mask(shortRaw.format(n)) }
  const today = new Date(Date.now() + 7 * 3600e3).toISOString().slice(0, 10)
  const WEEK = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min']

  // per-day totals for the open month (transfers are neither)
  const days = $derived.by(() => {
    const [y, m] = s.month.split('-').map(Number)
    const n = new Date(Date.UTC(y, m, 0)).getUTCDate()
    const list = Array.from({ length: n }, (_, i) => ({ date: `${s.month}-${String(i + 1).padStart(2, '0')}`, out: 0, in: 0, up: 0, txs: [] }))
    for (const t of s.txs) {
      const d = list[Number(t.occurred_at.slice(8, 10)) - 1]
      if (!d || t.occurred_at.slice(0, 7) !== s.month) continue
      d.txs.push(t)
      if (t.type === 'out' && t.category !== 'Investasi') d.out += t.amount // saving, not spending (as in Budget/Analitik)
      if (t.type === 'in') d.in += t.amount
    }
    for (const u of s.budget?.month === s.month ? s.budget.upcoming : []) {
      const d = list[Number(u.date.slice(8, 10)) - 1]
      if (d && u.date.startsWith(s.month) && u.date >= today && u.type === 'out') d.up += u.amount
    }
    return list
  })
  // Monday-first offset of day 1
  const lead = $derived((new Date(s.month + '-01T00:00:00Z').getUTCDay() + 6) % 7)
  const max = $derived(Math.max(1, ...days.map((d) => d.out)))
  const past = $derived(days.filter((d) => d.date <= today))
  const spent = $derived(days.reduce((n, d) => n + d.out, 0))
  const noSpend = $derived(past.filter((d) => !d.out).length)

  let picked = $state(null)
  const sel = $derived(days.find((d) => d.date === picked) ?? days.find((d) => d.date === today) ?? null)
</script>

<PageHead title="Kalender" />
<ViewToggle active="calendar" />
<MonthBar />

<section class="card tiles">
  <div><span>Keluar</span><b>{fmtIDR.format(spent)}</b></div>
  <div><span>Rata-rata / hari</span><b>{fmtIDR.format(past.length ? spent / past.length : 0)}</b></div>
  <div><span>Tanpa belanja</span><b>{noSpend} / {past.length}</b></div>
</section>

<section class="card">
<div class="grid">
  {#each WEEK as w}<span class="wd">{w}</span>{/each}
  {#each { length: lead } as _}<span></span>{/each}
  {#each days as d (d.date)}
    <button
      class="day"
      class:today={d.date === today}
      class:sel={sel?.date === d.date}
      aria-pressed={sel?.date === d.date}
      class:future={d.date > today}
      style:--heat={d.out ? Math.round(12 + 58 * Math.sqrt(d.out / max)) + '%' : '0%'}
      aria-label="{fmtDay.format(new Date(d.date + 'T12:00:00+07:00'))}: keluar {fmtIDR.format(d.out)}{d.in ? ', masuk ' + fmtIDR.format(d.in) : ''}{d.up ? ', tagihan ' + fmtIDR.format(d.up) : ''}"
      onclick={() => (picked = d.date)}
    >
      <span class="n">{Number(d.date.slice(8))}{#if d.in}<i class="inc"></i>{/if}</span>
      {#if d.out}<span class="amt">{short.format(d.out)}</span>{/if}
      {#if d.up}<span class="up">~{short.format(d.up)}</span>{/if}
    </button>
  {/each}
</div>
<p class="legend"><span class="sw"></span>makin gelap = makin besar pengeluaran · <i class="inc"></i> ada pemasukan · ~ tagihan berulang · Investasi nggak dihitung</p>
</section>

{#if sel}
  <section class="card list day-list">
  <h3>
    <span>{fmtDay.format(new Date(sel.date + 'T12:00:00+07:00'))}</span>
    <span class="tot">{sel.out ? '−' + fmtIDR.format(sel.out) : ''}{sel.in ? ' +' + fmtIDR.format(sel.in) : ''}</span>
  </h3>
  {#each sel.txs as tx (tx.id)}
    <TxRow {tx} onclick={() => open(tx)} />
  {:else}
    <p class="empty">{sel.up ? `Tagihan berulang ${fmtIDR.format(sel.up)} dijadwalkan.` : 'Nggak ada transaksi.'}</p>
  {/each}
  </section>
{/if}

<style>
  .card {
    margin-bottom: 12px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
  }
  .tiles div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .tiles span {
    color: var(--muted);
    font-size: 12px;
  }
  .tiles b {
    font-size: 14px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  @media (max-width: 459px) {
    .tiles {
      grid-template-columns: 1fr 1fr;
    }
    .tiles div:last-child {
      grid-column: span 2;
    }
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 4px;
  }
  .wd {
    text-align: center;
    font-size: 11px;
    color: var(--muted);
    padding-bottom: 2px;
  }
  .day {
    min-width: 0;
    min-height: 58px;
    padding: 5px 3px 5px 5px;
    border: 1px solid transparent;
    border-radius: 10px;
    background: color-mix(in srgb, var(--out) var(--heat), var(--bg));
    color: var(--text);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    font: inherit;
    text-align: left;
    overflow: hidden;
  }
  .day span {
    max-width: 100%;
    white-space: nowrap;
    letter-spacing: -0.3px;
  }
  .future {
    background: none;
    border-color: var(--line);
  }
  .today .n {
    color: var(--accent);
    font-weight: 700;
  }
  .sel {
    border-color: var(--accent);
    box-shadow: inset 0 0 0 1px var(--accent);
  }
  .n {
    font-size: 12px;
    display: flex;
    align-items: center;
    gap: 3px;
  }
  .amt {
    font-size: 11px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .up {
    font-size: 11px;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .inc {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--in);
  }
  .legend {
    font-size: 12px;
    color: var(--muted);
    margin: 8px 0 0;
  }
  .sw {
    display: inline-block;
    width: 18px;
    height: 10px;
    border-radius: 3px;
    margin-right: 4px;
    vertical-align: -1px;
    background: linear-gradient(90deg, color-mix(in srgb, var(--out) 12%, var(--surface)), color-mix(in srgb, var(--out) 70%, var(--surface)));
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
  .tot {
    font-variant-numeric: tabular-nums;
    color: var(--text);
  }
  @media (min-width: 900px) {
    .day {
      min-height: 84px;
      padding: 8px;
    }
    .amt {
      font-size: 13px;
    }
    .tiles b {
      font-size: 18px;
    }
  }
</style>
