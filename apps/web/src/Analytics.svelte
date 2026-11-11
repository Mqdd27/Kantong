<script>
  import { s, view, fmtIDR, fmtMonth, shiftMonth, mask } from './lib/store.svelte.js'
  import { api } from './lib/api.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'

  const RANGES = [3, 6, 12] // months shown; the AI card uses the same range
  let N = $state(6)
  const SAVING = new Set(['Investasi']) // out-categories that are saving, not spending
  const compactRaw = new Intl.NumberFormat('id-ID', { notation: 'compact', maximumFractionDigits: 1 })
  const compact = { format: (n) => mask(compactRaw.format(n)) }
  const shortMonth = new Intl.DateTimeFormat('id-ID', { month: 'short', timeZone: 'UTC' })
  const mlabel = (m) => shortMonth.format(new Date(m + '-15T00:00:00Z'))

  let months = $state([]) // [{month, in, spend, save, byCat: Map}]
  let hover = $state(null)
  let w = $state(600)

  $effect(() => {
    const end = s.month
    const list = Array.from({ length: N }, (_, i) => shiftMonth(end, i - N + 1))
    Promise.all(list.map((m) => api(`/summary?month=${m}`)))
      .then((res) => {
        months = res.map((r, i) => {
          const byCat = new Map()
          let save = 0
          for (const l of r.by_category) {
            if (l.type !== 'out') continue
            if (SAVING.has(l.category)) save += l.total
            else byCat.set(l.category, l.total)
          }
          return { month: list[i], in: r.in, spend: r.out - save, save, byCat }
        })
      })
      .catch(() => {})
  })

  // "Pendapat AI": on demand (a Hermes turn is slow and costs tokens); cached per month while the page is open
  let insights = $state({}) // `${month}|${months}` -> text
  let insightBusy = $state(false)
  let insightErr = $state('')
  const insightKey = $derived(`${s.month}|${N}`)
  const insight = $derived(insights[insightKey])
  async function askInsight() {
    const key = insightKey
    const month = s.month
    insightBusy = true
    insightErr = ''
    try {
      const r = await api('/insight', { method: 'POST', body: { month, months: N } })
      insights[key] = r.text
    } catch (e) {
      insightErr = e.message
    } finally {
      insightBusy = false
    }
  }
  // changing month or range clears the error, and the cache is keyed by both, so the card starts empty again
  $effect(() => {
    insightKey
    insightErr = ''
  })

  const rate = (m) => (m.in ? (m.in - m.spend) / m.in : null)
  const withData = $derived(months.filter((m) => m.in || m.spend))
  const avgSpend = $derived(withData.length ? withData.reduce((n, m) => n + m.spend, 0) / withData.length : 0)
  const avgRate = $derived.by(() => {
    const r = withData.map(rate).filter((x) => x != null)
    return r.length ? r.reduce((a, b) => a + b, 0) / r.length : null
  })

  // this month vs average of the previous months, per category
  const cats = $derived.by(() => {
    if (!months.length) return []
    const cur = months.at(-1).byCat
    const prev = months.slice(0, -1)
    const names = new Set([...cur.keys(), ...prev.flatMap((m) => [...m.byCat.keys()])])
    return [...names]
      .map((c) => ({ c, now: cur.get(c) || 0, avg: prev.reduce((n, m) => n + (m.byCat.get(c) || 0), 0) / prev.length }))
      .sort((a, b) => Math.max(b.now, b.avg) - Math.max(a.now, a.avg))
      .slice(0, 8)
  })
  const catMax = $derived(Math.max(1, ...cats.map((x) => Math.max(x.now, x.avg))))

  // grouped bars
  const H = 220, padB = 24, padT = 8, padR = 48
  const geo = $derived.by(() => {
    const max = Math.max(1, ...months.flatMap((m) => [m.in, m.spend])) * 1.1
    const band = (w - padR) / Math.max(1, months.length)
    const bw = Math.min(28, (band - 18) / 2)
    const y = (v) => padT + (1 - v / max) * (H - padT - padB)
    const ticks = [0, 1, 2, 3].map((t) => (max * t) / 3)
    return { band, bw, y, ticks }
  })
  const bar = (x, y0, y1) => {
    // 4px rounded top, square at the baseline
    const h = Math.max(0, y1 - y0), r = Math.min(4, h)
    return `M${x},${y1}V${y0 + r}Q${x},${y0} ${x + r},${y0}H${x + geo.bw - r}Q${x + geo.bw},${y0} ${x + geo.bw},${y0 + r}V${y1}Z`
  }
</script>

<PageHead title="Analitik" />
<div class="rangebar">
  <p class="range muted">{months.length ? `${fmtMonth(months[0].month)} – ${fmtMonth(s.month)}` : ''}</p>
  <div class="seg" role="group" aria-label="Rentang waktu">
    {#each RANGES as r}
      <button type="button" aria-pressed={N === r} onclick={() => (N = r)}>{r} bln</button>
    {/each}
  </div>
</div>

<section class="card tiles">
  <div><span>Rata-rata pengeluaran / bulan</span><b>{fmtIDR.format(avgSpend)}</b></div>
  <div><span>Rata-rata tingkat tabungan</span><b>{avgRate == null ? '–' : Math.round(avgRate * 100) + '%'}</b></div>
  <div><span>Diinvestasikan ({N} bln)</span><b>{fmtIDR.format(months.reduce((n, m) => n + m.save, 0))}</b></div>
</section>

<div class="cols">
  <section class="card">
    <h3>Pemasukan vs pengeluaran</h3>
    <div class="legend"><span><i class="k k1"></i>Masuk</span><span><i class="k k2"></i>Keluar</span></div>
    <div class="plot" bind:clientWidth={w}>
      <svg width={w} height={H} role="img" aria-label="Pemasukan dan pengeluaran per bulan, lihat tabel di bawah">
        {#each geo.ticks as t}
          <line class="grid" x1="0" x2={w - padR} y1={geo.y(t)} y2={geo.y(t)} />
          <text class="tick" x={w - padR + 6} y={geo.y(t) + 4}>{compact.format(t)}</text>
        {/each}
        {#each months as m, i}
          {@const x0 = i * geo.band + (geo.band - geo.bw * 2 - 2) / 2}
          <g
            role="presentation"
            class:dim={hover != null && hover !== i}
            onpointerenter={(e) => e.pointerType === 'mouse' && (hover = i)}
            onpointerleave={(e) => e.pointerType === 'mouse' && (hover = null)}
            onclick={() => (hover = hover === i ? null : i)}
          >
            <rect class="hit" x={i * geo.band} y="0" width={geo.band} height={H} />
            <path class="b1" d={bar(x0, geo.y(m.in), geo.y(0))} />
            <path class="b2" d={bar(x0 + geo.bw + 2, geo.y(m.spend), geo.y(0))} />
            <text class="tick" x={i * geo.band + geo.band / 2} y={H - 6} text-anchor="middle">{mlabel(m.month)}</text>
          </g>
        {/each}
      </svg>
      {#if hover != null && months[hover]}
        {@const m = months[hover]}
        <div class="tip" style:left="{Math.min(hover * geo.band + geo.band / 2, w - 180)}px">
          <div>{fmtMonth(m.month)}</div>
          <div><i class="k k1"></i><b>{fmtIDR.format(m.in)}</b> Masuk</div>
          <div><i class="k k2"></i><b>{fmtIDR.format(m.spend)}</b> Keluar</div>
          {#if rate(m) != null}<div>Tabungan <b>{Math.round(rate(m) * 100)}%</b></div>{/if}
        </div>
      {/if}
    </div>
    <table>
      <thead><tr><th>Bulan</th><th>Masuk</th><th>Keluar</th><th>Tabungan</th></tr></thead>
      <tbody>
        {#each months as m}
          <tr>
            <td>{mlabel(m.month)}</td>
            <td>{compact.format(m.in)}</td>
            <td>{compact.format(m.spend)}</td>
            <td class:pos={rate(m) > 0} class:neg={rate(m) < 0}>{rate(m) == null ? '–' : Math.round(rate(m) * 100) + '%'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    <p class="muted small">Keluar = pengeluaran tanpa kategori Investasi. Tabungan = (masuk − keluar) / masuk.</p>
  </section>

  <section class="card">
    <h3>Bulan ini vs rata-rata {N - 1} bulan sebelumnya</h3>
    <ul class="cats">
      {#each cats as x (x.c)}
        {@const diff = x.avg ? (x.now - x.avg) / x.avg : null}
        {@const look = catLook(x.c)}
        <li>
          <span class="avatar sm" style:--hue={look.hue}><Icon name={look.icon} size={18} /></span>
          <div class="what">
          <div class="line">
            <span>{x.c}</span>
            <span class="amt">
              {fmtIDR.format(x.now)}
              {#if diff != null}<small class:neg={diff > 0.1} class:pos={diff < -0.1}><Icon name={diff > 0 ? 'up' : 'down'} size={12} label={diff > 0 ? 'naik' : 'turun'} />{Math.abs(Math.round(diff * 100))}%</small>{/if}
            </span>
          </div>
          <div class="track">
            <span class="grow" style:width="{(x.now / catMax) * 100}%"></span>
            {#if x.avg}<i class="grow" style:left="{(x.avg / catMax) * 100}%" title="Rata-rata {fmtIDR.format(x.avg)}"></i>{/if}
          </div>
          <div class="muted small">rata-rata {fmtIDR.format(x.avg)}</div>
          </div>
        </li>
      {:else}
        <li class="muted">Belum ada data.</li>
      {/each}
    </ul>
  </section>
</div>

{#if s.chat}
  <section class="card insight">
    <h3>Pendapat AI</h3>
    {#if insight && view.hide}
      <p class="muted">Teks disembunyikan karena nominal disembunyikan. Tap ikon mata untuk menampilkan.</p>
    {:else if insight}
      <p class="text">{insight}</p>
    {:else if insightBusy}
      <p class="muted" role="status">AI sedang membaca datamu…</p>
    {:else}
      <p class="muted">Komentar singkat soal angka {fmtMonth(months[0]?.month ?? s.month)} – {fmtMonth(s.month)}.</p>
    {/if}
    {#if insightErr}<p class="err" role="alert">{insightErr}</p>{/if}
    {#if !insightBusy}
      <button onclick={askInsight}>{insightErr ? 'Ulangi' : insight ? 'Minta lagi' : 'Minta pendapat AI'}</button>
    {/if}
  </section>
{/if}

<style>
  .rangebar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin: -8px 0 12px;
  }
  .range {
    margin: 0;
  }
  .rangebar .seg {
    flex: none;
  }
  .rangebar .seg button {
    padding: 6px 12px;
    font-size: 13px;
  }
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .small {
    font-size: 12px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
    margin-bottom: 12px;
  }
  .tiles div {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .tiles span {
    color: var(--muted);
    font-size: 12px;
  }
  .tiles b {
    font-size: 15px;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  .insight {
    margin-top: 12px;
  }
  .insight h3 {
    margin-bottom: 8px;
  }
  .insight p {
    margin: 0 0 10px;
  }
  .insight .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    text-align: justify;
    line-height: 1.5;
  }
  .insight .err {
    color: var(--out);
    font-size: 14px;
  }
  .cols {
    display: grid;
    gap: 12px;
  }
  .cols > * {
    min-width: 0;
  }
  @media (min-width: 900px) {
    .cols {
      grid-template-columns: 3fr 2fr;
      gap: 16px;
      align-items: start;
    }
    .tiles b {
      font-size: 20px;
    }
  }
  h3 {
    margin: 0 0 12px;
    font-size: 16px;
    font-weight: 650;
  }
  .legend {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: var(--muted);
    margin-bottom: 4px;
  }
  .k {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 3px;
    margin-right: 6px;
    vertical-align: -1px;
  }
  .k1 {
    background: var(--series-1);
  }
  .k2 {
    background: var(--series-2);
  }
  .plot {
    position: relative;
    overflow: hidden;
  }
  svg {
    display: block;
  }
  .grid {
    stroke: var(--line);
  }
  .tick {
    fill: var(--muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .hit {
    fill: transparent;
  }
  .b1 {
    fill: var(--series-1);
  }
  .b2 {
    fill: var(--series-2);
  }
  .dim path {
    opacity: 0.35;
  }
  .tip {
    position: absolute;
    top: 4px;
    transform: translateX(8px);
    pointer-events: none;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 8px 10px;
    font-size: 12px;
    color: var(--muted);
    display: grid;
    gap: 2px;
    min-width: 160px;
  }
  .tip b {
    color: var(--text);
    margin-right: 4px;
    font-variant-numeric: tabular-nums;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 12px;
    font-size: 13px;
    font-variant-numeric: tabular-nums;
  }
  th,
  td {
    text-align: right;
    padding: 6px 4px;
    border-bottom: 1px solid var(--line);
  }
  th:first-child,
  td:first-child {
    text-align: left;
  }
  th {
    color: var(--muted);
    font-weight: 500;
  }
  .pos {
    color: var(--in);
  }
  .neg {
    color: var(--out);
  }
  .cats {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 16px;
  }
  .cats li {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .what {
    flex: 1;
    min-width: 0;
  }
  .line {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: 14px;
  }
  .amt {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .amt small {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-size: 12px;
    margin-left: 4px;
  }
  .track {
    position: relative;
    height: 8px;
    border-radius: 4px;
    background: var(--bg);
    margin: 6px 0 2px;
  }
  .track span {
    display: block;
    height: 100%;
    border-radius: 4px;
    background: var(--series-2);
  }
  .track i {
    position: absolute;
    top: -3px;
    bottom: -3px;
    width: 2px;
    margin-left: -1px;
    background: var(--text);
  }
</style>
