<script>
  import { s, view, fmtIDR, fmtMonth, shiftMonth, mask } from './lib/store.svelte.js'
  import { api } from './lib/api.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import Icon from './Icon.svelte'

  const RANGES = [3, 6, 12] // trend section; the AI card uses the same range
  let N = $state(6)
  const AVG = 3 // per-category delta compares with the average of this many previous months
  const SAVING = new Set(['Investasi']) // out-categories that are saving, not spending
  const compactRaw = new Intl.NumberFormat('id-ID', { notation: 'compact', maximumFractionDigits: 1 })
  const compact = { format: (n) => mask(compactRaw.format(n)) }
  const shortMonth = new Intl.DateTimeFormat('id-ID', { month: 'short', timeZone: 'UTC' })
  const mlabel = (m) => shortMonth.format(new Date(m + '-15T00:00:00Z'))
  const pct = (x) => Math.round(x * 100) + '%'

  // the last 12 months ending at the selected month: the trend slices N of them, deltas use the previous AVG
  let all = $state([]) // [{month, in, spend, save, byCat: Map, raw}]
  let loading = $state(true)
  $effect(() => {
    const end = s.month
    const list = Array.from({ length: 12 }, (_, i) => shiftMonth(end, i - 11))
    loading = true
    Promise.all(list.map((m) => api(`/summary?month=${m}`)))
      .then((res) => {
        if (end !== s.month) return
        all = res.map((r, i) => {
          const byCat = new Map()
          let save = 0
          for (const l of r.by_category) {
            if (l.type !== 'out') continue
            if (SAVING.has(l.category)) save += l.total
            else byCat.set(l.category, l.total)
          }
          return { month: list[i], in: r.in, spend: r.out - save, save, byCat, raw: r.by_category }
        })
      })
      .catch(() => {})
      .finally(() => (loading = false))
  })
  const cur = $derived(all.at(-1))
  const last = $derived(all.at(-2))
  const months = $derived(all.slice(-N))
  const rate = (m) => (m?.in ? (m.in - m.spend) / m.in : null)

  // ---- this month: summary ----
  const delta = (a, b) => (b ? (a - b) / b : null)

  // ---- this month: cashflow (sankey) ----
  const topN = (lines, n) => {
    const a = lines.map((l) => ({ c: l.category, v: l.total })).sort((x, y) => y.v - x.v)
    const rest = a.slice(n).reduce((t, x) => t + x.v, 0)
    return rest ? [...a.slice(0, n), { c: 'Lainnya', v: rest }] : a
  }
  let sw = $state(600)
  const NW = 10, SLOT = 30, GAP = 6, TOP = 30
  const sankey = $derived.by(() => {
    const raw = cur?.raw ?? []
    const ins = topN(raw.filter((l) => l.type === 'in'), 4).map((x) => ({ ...x, hue: 'var(--in)' }))
    const outs = topN(raw.filter((l) => l.type === 'out'), 8).map((x) => ({ ...x, hue: x.c === 'Lainnya' ? 'var(--muted)' : catLook(x.c).hue }))
    const tin = ins.reduce((t, x) => t + x.v, 0), tout = outs.reduce((t, x) => t + x.v, 0)
    if (tin > tout) outs.push({ c: 'Surplus', v: tin - tout, hue: 'var(--in)' })
    if (tout > tin) ins.push({ c: 'Deficit', v: tout - tin, hue: 'var(--out)' })
    const T = Math.max(tin, tout)
    if (!T) return null
    const k = 220 / T
    // a small node still gets a label-sized slot, so labels never overlap
    const stack = (nodes) => {
      let y = TOP
      return nodes.map((n) => {
        const h = Math.max(2, n.v * k), slot = Math.max(h, SLOT)
        const o = { ...n, y: y + (slot - h) / 2, h }
        y += slot + GAP
        return o
      })
    }
    const L = stack(ins), R = stack(outs)
    const height = Math.max(TOP + T * k, ...[...L, ...R].map((n) => n.y + n.h)) + 4
    const mx = (sw - NW) / 2, my = TOP + (height - TOP - 4 - T * k) / 2
    const ribbon = (x1, a, x2, b, t) => {
      const c = (x1 + x2) / 2
      return `M${x1},${a}C${c},${a} ${c},${b} ${x2},${b}L${x2},${b + t}C${c},${b + t} ${c},${a + t} ${x1},${a + t}Z`
    }
    let li = my, ri = my
    const links = [
      ...L.map((n) => ({ n, d: ribbon(NW, n.y, mx, (li += n.v * k) - n.v * k, n.v * k) })),
      ...R.map((n) => ({ n, d: ribbon(mx + NW, (ri += n.v * k) - n.v * k, sw - NW, n.y, n.v * k) })),
    ]
    return { L, R, links, height, mx, my, mh: T * k, T }
  })

  // ---- this month: spending by category, share + change vs the previous months' average ----
  const R0 = 64, C = 2 * Math.PI * R0
  const spending = $derived.by(() => {
    if (!cur) return { total: 0, parts: [] }
    const prev = all.slice(-1 - AVG, -1)
    const avg = (c) => prev.reduce((n, m) => n + (m.byCat.get(c) || 0), 0) / (prev.length || 1)
    const sorted = [...cur.byCat].sort((a, b) => b[1] - a[1])
    const total = sorted.reduce((n, [, v]) => n + v, 0)
    const parts = sorted.slice(0, 6).map(([c, v]) => ({ c, v, avg: avg(c), hue: catLook(c).hue, icon: catLook(c).icon }))
    const rest = sorted.slice(6)
    if (rest.length) parts.push({ c: `Lainnya (${rest.length})`, v: rest.reduce((n, [, v]) => n + v, 0), avg: null, hue: 'var(--muted)', icon: 'grid' })
    let acc = 0
    return { total, parts: parts.map((p) => ({ ...p, from: (acc += p.v) - p.v })) }
  })
  let active = $state(null) // category hovered in the donut or the list

  // ---- this month: daily pace, cumulative spending vs last month ----
  let prevTxs = $state([])
  $effect(() => {
    const m = shiftMonth(s.month, -1)
    api(`/transactions?month=${m}`).then((r) => m === shiftMonth(s.month, -1) && (prevTxs = r)).catch(() => (prevTxs = []))
  })
  const daysIn = (m) => new Date(Date.UTC(+m.slice(0, 4), +m.slice(5, 7), 0)).getUTCDate()
  const cum = (txs, n) => {
    const d = Array(n).fill(0)
    for (const t of txs) if (t.type === 'out' && !SAVING.has(t.category)) d[+t.occurred_at.slice(8, 10) - 1] += t.amount
    for (let i = 1; i < n; i++) d[i] += d[i - 1]
    return d
  }
  let dw = $state(400)
  const DH = 180, padT = 8, padB = 22, padR = 44
  const daily = $derived.by(() => {
    const pm = shiftMonth(s.month, -1)
    const n = Math.max(daysIn(s.month), daysIn(pm))
    const now = new Date()
    const thisMonth = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
    const upTo = s.month === thisMonth ? now.getDate() : s.month > thisMonth ? 0 : daysIn(s.month)
    const c = cum(s.txs, n).slice(0, upTo)
    const p = cum(prevTxs, daysIn(pm))
    const max = Math.max(1, ...c, ...p) * 1.1
    const x = (i) => (n < 2 ? 0 : (i / (n - 1)) * (dw - padR))
    const y = (v) => padT + (1 - v / max) * (DH - padT - padB)
    const line = (a) => a.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')
    const area = (a) => (a.length ? `${line(a)}L${x(a.length - 1).toFixed(1)},${y(0)}L0,${y(0)}Z` : '')
    const ticks = [0, 1, 2, 3].map((t) => (max * t) / 3)
    const day = c.length // compare at the same day of month
    const gap = day ? c[day - 1] - (p[Math.min(day, p.length) - 1] ?? 0) : null
    return { n, cur: c, prev: p, x, y, ticks, curLine: line(c), curArea: area(c), prevLine: line(p), day, gap }
  })
  let dHover = $state(null)
  const dMove = (e) => {
    const r = e.currentTarget.getBoundingClientRect()
    dHover = Math.max(0, Math.min(daily.n - 1, Math.round(((e.clientX - r.left) / (dw - padR)) * (daily.n - 1))))
  }

  // ---- trend: income vs expense ----
  let tw = $state(600)
  let hover = $state(null)
  const H = 200, bpadB = 22
  const geo = $derived.by(() => {
    const max = Math.max(1, ...months.flatMap((m) => [m.in, m.spend])) * 1.1
    const band = (tw - padR) / Math.max(1, months.length)
    const bw = Math.max(4, Math.min(24, (band - 14) / 2))
    const y = (v) => padT + (1 - v / max) * (H - padT - bpadB)
    const ticks = [0, 1, 2, 3].map((t) => (max * t) / 3)
    return { band, bw, y, ticks }
  })
  const bar = (x, y0, y1) => {
    // 4px rounded top, square at the baseline
    const h = Math.max(0, y1 - y0), r = Math.min(4, h, geo.bw / 2)
    return `M${x},${y1}V${y0 + r}Q${x},${y0} ${x + r},${y0}H${x + geo.bw - r}Q${x + geo.bw},${y0} ${x + geo.bw},${y0 + r}V${y1}Z`
  }
  const withData = $derived(months.filter((m) => m.in || m.spend))
  const avgSpend = $derived(withData.length ? withData.reduce((n, m) => n + m.spend, 0) / withData.length : 0)
  const avgRate = $derived.by(() => {
    const r = withData.map(rate).filter((x) => x != null)
    return r.length ? r.reduce((a, b) => a + b, 0) / r.length : null
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
</script>

{#snippet change(d, upIsBad = true)}
  {#if d != null && Math.abs(d) >= 0.01}
    <small class="chg" class:bad={upIsBad ? d > 0 : d < 0} class:good={upIsBad ? d < 0 : d > 0}>
      <Icon name={d > 0 ? 'up' : 'down'} size={12} label={d > 0 ? 'naik' : 'turun'} />{pct(Math.abs(d))}
    </small>
  {/if}
{/snippet}

<PageHead title="Analitik" />
<MonthBar />

<section class="card stats" aria-label="Ringkasan {fmtMonth(s.month)}">
  <div>
    <span>Income</span>
    <b class="pos">{fmtIDR.format(cur?.in ?? 0)}</b>
    {@render change(delta(cur?.in, last?.in), false)}
  </div>
  <div>
    <span>Spending</span>
    <b>{fmtIDR.format(cur?.spend ?? 0)}</b>
    {@render change(delta(cur?.spend, last?.spend))}
  </div>
  <div>
    <span>Savings rate</span>
    <b>{rate(cur) == null ? '–' : pct(rate(cur))}</b>
    {#if cur?.save}<small class="muted">{compact.format(cur.save)} diinvestasikan</small>{/if}
  </div>
</section>
<p class="note">Persentase dibanding {last ? fmtMonth(last.month) : 'bulan lalu'}.</p>

<h2 class="sec">Bulan ini</h2>

<section class="card">
  <header class="ch">
    <h3>Cashflow</h3>
    <span class="muted">Dari mana masuk, ke mana keluar</span>
  </header>
  <div class="plot" bind:clientWidth={sw}>
    {#if sankey}
      {@const g = sankey}
      <svg class="sankey" width={sw} height={g.height} role="img" aria-label="Cashflow: pemasukan {fmtIDR.format(g.T)} dibagi ke kategori pengeluaran">
        {#each g.links as l}
          <path class="link" d={l.d} fill={l.n.hue}><title>{l.n.c}: {fmtIDR.format(l.n.v)}</title></path>
        {/each}
        <rect x={g.mx} y={g.my} width={NW} height={g.mh} rx="2" fill="var(--series-1)" />
        <text class="nl" x={g.mx + NW / 2} y={g.my - 18} text-anchor="middle">Total</text>
        <text class="tick" x={g.mx + NW / 2} y={g.my - 5} text-anchor="middle">{compact.format(g.T)}</text>
        {#each g.L as n}
          <rect x="0" y={n.y} width={NW} height={n.h} rx="2" fill={n.hue} />
          <text class="nl" x={NW + 6} y={n.y + n.h / 2 - 2}>{n.c}</text>
          <text class="tick" x={NW + 6} y={n.y + n.h / 2 + 11}>{compact.format(n.v)}</text>
        {/each}
        {#each g.R as n}
          <rect x={sw - NW} y={n.y} width={NW} height={n.h} rx="2" fill={n.hue} />
          <text class="nl" x={sw - NW - 6} y={n.y + n.h / 2 - 2} text-anchor="end">{n.c}</text>
          <text class="tick" x={sw - NW - 6} y={n.y + n.h / 2 + 11} text-anchor="end">{compact.format(n.v)}</text>
        {/each}
      </svg>
    {:else}
      <p class="empty">{loading ? 'Memuat…' : `Belum ada transaksi di ${fmtMonth(s.month)}.`}</p>
    {/if}
  </div>
</section>

<div class="pair">
  <section class="card">
    <header class="ch">
      <h3>Spending</h3>
      <span class="muted">vs rata-rata {AVG} bulan sebelumnya</span>
    </header>
    {#if spending.total}
      <div class="spend">
        <svg class="donut" width="160" height="160" viewBox="0 0 180 180" role="img" aria-label="Porsi pengeluaran per kategori, lihat daftar">
          <g transform="rotate(-90 90 90)">
            {#each spending.parts as p (p.c)}
              <circle
                role="presentation"
                cx="90" cy="90" r={R0} fill="none" stroke={p.hue}
                stroke-width={active === p.c ? 30 : 24}
                class:faded={active && active !== p.c}
                stroke-dasharray="{Math.max(0, (p.v / spending.total) * C - 2)} {C}"
                stroke-dashoffset={-(p.from / spending.total) * C}
                onpointerenter={() => (active = p.c)}
                onpointerleave={() => (active = null)}
              />
            {/each}
          </g>
          {#each [spending.parts.find((p) => p.c === active)] as a}
            <text x="90" y="84" text-anchor="middle" class="tick">{a ? a.c : 'Total'}</text>
            <text x="90" y="104" text-anchor="middle" class="total">{compact.format(a ? a.v : spending.total)}</text>
          {/each}
        </svg>
        <ul class="parts">
          {#each spending.parts as p (p.c)}
            <li class:faded={active && active !== p.c} onpointerenter={() => (active = p.c)} onpointerleave={() => (active = null)}>
              <span class="avatar sm" style:--hue={p.hue}><Icon name={p.icon} size={16} /></span>
              <span class="nm">{p.c}<small class="muted">{pct(p.v / spending.total)}</small></span>
              <span class="amt">{fmtIDR.format(p.v)}{#if p.avg}{@render change(delta(p.v, p.avg))}{/if}</span>
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      <p class="empty">{loading ? 'Memuat…' : 'Belum ada pengeluaran.'}</p>
    {/if}
    <p class="note">Tanpa kategori Investasi.</p>
  </section>

  <section class="card">
    <header class="ch">
      <h3>Daily pace</h3>
      <span class="muted">Pengeluaran kumulatif</span>
    </header>
    {#if daily.gap != null && daily.prev.length}
      <p class="verdict" class:bad={daily.gap > 0} class:good={daily.gap <= 0}>
        <b>{fmtIDR.format(Math.abs(daily.gap))}</b>
        {daily.gap > 0 ? 'lebih boros' : 'lebih hemat'} dari {fmtMonth(shiftMonth(s.month, -1))} di tanggal {daily.day}
      </p>
    {/if}
    <div class="legend"><span><i class="k k2"></i>{fmtMonth(s.month)}</span><span><i class="k prev"></i>{fmtMonth(shiftMonth(s.month, -1))}</span></div>
    <div class="plot" bind:clientWidth={dw}>
      <svg width={dw} height={DH} role="img" aria-label="Pengeluaran kumulatif {fmtMonth(s.month)} {fmtIDR.format(daily.cur.at(-1) ?? 0)}, bulan lalu {fmtIDR.format(daily.prev.at(-1) ?? 0)}"
        onpointermove={dMove} onpointerleave={() => (dHover = null)}>
        {#each daily.ticks as t}
          <line class="grid" x1="0" x2={dw - padR} y1={daily.y(t)} y2={daily.y(t)} />
          <text class="tick" x={dw - padR + 6} y={daily.y(t) + 4}>{compact.format(t)}</text>
        {/each}
        {#each [1, 8, 15, 22, 29] as d}
          {#if d <= daily.n}<text class="tick" x={daily.x(d - 1)} y={DH - 6} text-anchor={d === 1 ? 'start' : 'middle'}>{d}</text>{/if}
        {/each}
        <path class="lprev" d={daily.prevLine} />
        <path class="acur" d={daily.curArea} />
        <path class="lcur" d={daily.curLine} />
        {#if daily.day}<circle class="dot" cx={daily.x(daily.day - 1)} cy={daily.y(daily.cur[daily.day - 1])} r="4" />{/if}
        {#if dHover != null}<line class="cursor" x1={daily.x(dHover)} x2={daily.x(dHover)} y1={padT} y2={DH - padB} />{/if}
      </svg>
      {#if dHover != null}
        <div class="tip" style:left="{Math.min(daily.x(dHover), dw - 180)}px">
          <div>Tanggal {dHover + 1}</div>
          {#if daily.cur[dHover] != null}<div><i class="k k2"></i><b>{fmtIDR.format(daily.cur[dHover])}</b> bulan ini</div>{/if}
          {#if daily.prev[dHover] != null}<div><i class="k prev"></i><b>{fmtIDR.format(daily.prev[dHover])}</b> bulan lalu</div>{/if}
        </div>
      {/if}
    </div>
  </section>
</div>

<div class="sechead">
  <h2 class="sec">Tren</h2>
  <div class="seg range" role="group" aria-label="Rentang tren">
    {#each RANGES as r}
      <button type="button" aria-pressed={N === r} onclick={() => (N = r)}>{r} bln</button>
    {/each}
  </div>
</div>

<section class="card">
  <header class="ch">
    <h3>Income vs Expense</h3>
    <span class="muted">{months.length ? `${fmtMonth(months[0].month)} – ${fmtMonth(s.month)}` : ''}</span>
  </header>
  <div class="legend"><span><i class="k k1"></i>Income</span><span><i class="k k2"></i>Spending</span></div>
  <div class="plot" bind:clientWidth={tw}>
    <svg width={tw} height={H} role="img" aria-label="Income dan spending per bulan, lihat tabel">
      {#each geo.ticks as t}
        <line class="grid" x1="0" x2={tw - padR} y1={geo.y(t)} y2={geo.y(t)} />
        <text class="tick" x={tw - padR + 6} y={geo.y(t) + 4}>{compact.format(t)}</text>
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
          <text class="tick" class:now={i === months.length - 1} x={i * geo.band + geo.band / 2} y={H - 6} text-anchor="middle">{mlabel(m.month)}</text>
        </g>
      {/each}
    </svg>
    {#if hover != null && months[hover]}
      {@const m = months[hover]}
      <div class="tip" style:left="{Math.min(hover * geo.band + geo.band / 2, tw - 180)}px">
        <div>{fmtMonth(m.month)}</div>
        <div><i class="k k1"></i><b>{fmtIDR.format(m.in)}</b> Income</div>
        <div><i class="k k2"></i><b>{fmtIDR.format(m.spend)}</b> Spending</div>
        {#if rate(m) != null}<div>Savings rate <b>{pct(rate(m))}</b></div>{/if}
      </div>
    {/if}
  </div>
  <dl class="avgs">
    <div><dt>Rata-rata spending</dt><dd>{fmtIDR.format(avgSpend)}</dd></div>
    <div><dt>Rata-rata savings rate</dt><dd>{avgRate == null ? '–' : pct(avgRate)}</dd></div>
    <div><dt>Diinvestasikan</dt><dd>{fmtIDR.format(months.reduce((n, m) => n + m.save, 0))}</dd></div>
  </dl>
  <details>
    <summary>Lihat tabel</summary>
    <table>
      <thead><tr><th>Bulan</th><th>Income</th><th>Spending</th><th>Savings</th></tr></thead>
      <tbody>
        {#each months as m}
          <tr>
            <td>{mlabel(m.month)}</td>
            <td>{compact.format(m.in)}</td>
            <td>{compact.format(m.spend)}</td>
            <td class:pos={rate(m) > 0} class:neg={rate(m) < 0}>{rate(m) == null ? '–' : pct(rate(m))}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </details>
  <p class="note">Spending tanpa kategori Investasi. Savings rate = (income − spending) / income; investasi dihitung sebagai tabungan.</p>
</section>

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
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .note {
    color: var(--muted);
    font-size: 12px;
    margin: 8px 2px 0;
  }
  .empty {
    color: var(--muted);
    font-size: 14px;
    margin: 24px 0;
    text-align: center;
  }
  .pos,
  .good {
    color: var(--in);
  }
  .neg,
  .bad {
    color: var(--out);
  }

  /* summary */
  .stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
  }
  .stats div {
    display: grid;
    align-content: start;
    gap: 2px;
    min-width: 0;
  }
  .stats span {
    color: var(--muted);
    font-size: 12px;
  }
  .stats b {
    font-size: 16px;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  .stats .muted {
    font-size: 12px;
  }
  .chg {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-size: 12px;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
  }
  .chg.good {
    color: var(--in);
  }
  .chg.bad {
    color: var(--out);
  }

  /* sections */
  .sec {
    margin: 28px 2px 10px;
    font-size: 18px;
    font-weight: 700;
  }
  .sechead {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 12px;
  }
  .sechead .sec {
    margin-bottom: 10px;
  }
  .range {
    margin-bottom: 8px;
  }
  .range button {
    padding: 6px 12px;
    font-size: 13px;
  }
  .card + .card,
  .pair,
  .pair + .sechead + .card {
    margin-top: 12px;
  }
  .pair {
    display: grid;
    gap: 12px;
  }
  .pair > .card {
    min-width: 0;
    margin-top: 0;
  }
  .ch {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 2px 12px;
    margin-bottom: 12px;
  }
  h3 {
    margin: 0;
    font-size: 16px;
    font-weight: 650;
  }

  /* shared chart bits */
  svg {
    display: block;
  }
  .plot {
    position: relative;
    overflow: hidden;
  }
  .grid {
    stroke: var(--line);
  }
  .cursor {
    stroke: var(--muted);
    stroke-dasharray: 2 3;
  }
  .tick {
    fill: var(--muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .tick.now {
    fill: var(--text);
    font-weight: 600;
  }
  .legend {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: var(--muted);
    margin-bottom: 6px;
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
  .prev {
    background: var(--muted);
  }
  .tip {
    position: absolute;
    top: 4px;
    transform: translateX(8px);
    pointer-events: none;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 8px;
    box-shadow: 0 4px 16px rgb(16 24 40 / 0.12);
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

  /* cashflow */
  .link {
    opacity: 0.22;
    transition: opacity 0.15s;
  }
  .sankey:hover .link {
    opacity: 0.1;
  }
  .sankey .link:hover {
    opacity: 0.45;
  }
  .nl {
    fill: var(--text);
    font-size: 12px;
    font-weight: 600;
  }
  .sankey text {
    paint-order: stroke;
    stroke: var(--surface);
    stroke-width: 3px;
    stroke-linejoin: round;
  }

  /* spending */
  .spend {
    display: grid;
    justify-items: center;
    gap: 16px;
  }
  .donut circle {
    transition: stroke-width 0.15s, opacity 0.15s;
  }
  .faded {
    opacity: 0.35;
  }
  .total {
    fill: var(--text);
    font-size: 18px;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }
  .parts {
    list-style: none;
    padding: 0;
    margin: 0;
    width: 100%;
    display: grid;
    gap: 4px;
  }
  .parts li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 0;
    transition: opacity 0.15s;
  }
  .nm {
    flex: 1;
    min-width: 0;
    display: grid;
    font-size: 14px;
  }
  .nm small {
    font-size: 12px;
  }
  .amt {
    display: grid;
    justify-items: end;
    font-size: 14px;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  /* daily pace */
  .verdict {
    margin: 0 0 12px;
    font-size: 14px;
    color: var(--muted);
  }
  .verdict b {
    font-size: 18px;
    font-variant-numeric: tabular-nums;
    margin-right: 4px;
  }
  .verdict.bad b {
    color: var(--out);
  }
  .verdict.good b {
    color: var(--in);
  }
  .lcur,
  .lprev {
    fill: none;
    stroke-width: 2.5;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .lcur {
    stroke: var(--series-2);
  }
  .acur {
    fill: var(--series-2);
    opacity: 0.12;
  }
  .lprev {
    stroke: var(--muted);
    stroke-dasharray: 4 4;
  }
  .dot {
    fill: var(--series-2);
    stroke: var(--surface);
    stroke-width: 2;
  }

  /* trend */
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
  .avgs {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin: 16px 0 0;
    padding-top: 12px;
    border-top: 1px solid var(--line);
  }
  .avgs dt {
    color: var(--muted);
    font-size: 12px;
  }
  .avgs dd {
    margin: 2px 0 0;
    font-size: 14px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }
  details {
    margin-top: 12px;
  }
  summary {
    cursor: pointer;
    color: var(--accent);
    font-size: 13px;
    font-weight: 500;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    margin-top: 8px;
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

  /* AI */
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
    line-height: 1.5;
  }
  .insight .err {
    color: var(--out);
    font-size: 14px;
  }

  @media (min-width: 900px) {
    .stats b {
      font-size: 22px;
    }
    .pair {
      grid-template-columns: 1fr 1fr;
      gap: 16px;
      align-items: start;
    }
    .spend {
      grid-template-columns: auto 1fr;
      align-items: center;
    }
  }
</style>
