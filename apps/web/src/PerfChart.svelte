<script>
  import { fmtIDR, mask } from './lib/store.svelte.js'

  /** points: [{date: 'YYYY-MM-DD', total, invested}] sorted by date */
  let { points } = $props()

  const RANGES = [['1B', 30], ['3B', 90], ['1T', 365], ['Semua', 0]]
  let range = $state(90)
  let w = $state(600)
  let hover = $state(null) // index into shown

  const compactRaw = new Intl.NumberFormat('id-ID', { notation: 'compact', maximumFractionDigits: 1 })
  const compact = { format: (n) => mask(compactRaw.format(n)) }
  const fmtD = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: '2-digit', timeZone: 'UTC' })
  const day = (d) => fmtD.format(new Date(d + 'T00:00:00Z'))

  const shown = $derived.by(() => {
    if (!range) return points
    const from = new Date(Date.now() - range * 864e5).toISOString().slice(0, 10)
    return points.filter((p) => p.date >= from)
  })

  const H = 240, padL = 8, padR = 56, padT = 12, padB = 24
  const geo = $derived.by(() => {
    const n = shown.length
    const vals = shown.flatMap((p) => [p.total, p.invested])
    let lo = Math.min(...vals), hi = Math.max(...vals)
    const pad = (hi - lo) * 0.1 || hi * 0.05 || 1
    lo = Math.max(0, lo - pad)
    hi += pad
    const x = (i) => padL + (n < 2 ? 0 : (i / (n - 1)) * (w - padL - padR))
    const y = (v) => padT + (1 - (v - lo) / (hi - lo)) * (H - padT - padB)
    const line = (k) => shown.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p[k]).toFixed(1)}`).join('')
    const ticks = [0, 1, 2, 3].map((t) => lo + ((hi - lo) * t) / 3)
    return { x, y, ticks, total: line('total'), invested: line('invested'), base: y(lo) }
  })

  const first = $derived(shown[0])
  const last = $derived(shown.at(-1))
  const gain = $derived(last ? last.total - last.invested : 0)
  const change = $derived(first && last ? last.total - first.total : 0)

  function move(e) {
    const r = e.currentTarget.getBoundingClientRect()
    const n = shown.length
    const i = Math.round(((e.clientX - r.left - padL) / (w - padL - padR)) * (n - 1))
    hover = Math.max(0, Math.min(n - 1, i))
  }
  const hp = $derived(hover == null ? null : shown[hover])
</script>

<section class="card">
  <header>
    <div>
      <h3>Perkembangan aset</h3>
      {#if last}
        <p class="sub">
          <span class:pos={change >= 0} class:neg={change < 0}>{change >= 0 ? '+' : '−'}{fmtIDR.format(Math.abs(change))}</span>
          sejak {day(first.date)} · untung/rugi belum terealisasi
          <span class:pos={gain >= 0} class:neg={gain < 0}>{gain >= 0 ? '+' : '−'}{fmtIDR.format(Math.abs(gain))}</span>
        </p>
      {/if}
    </div>
    <div class="ranges" role="group" aria-label="Rentang waktu">
      {#each RANGES as [label, d]}
        <button class:on={range === d} aria-pressed={range === d} onclick={() => ((range = d), (hover = null))}>{label}</button>
      {/each}
    </div>
  </header>

  {#if shown.length < 2}
    <p class="empty">Grafik muncul setelah ada data minimal 2 hari. Nilai aset dicatat otomatis tiap hari.</p>
  {:else}
    <div class="legend">
      <span><i class="key k1"></i>Nilai</span>
      <span><i class="key k2"></i>Modal</span>
    </div>
    <div class="plot" bind:clientWidth={w}>
      <svg
        width={w}
        height={H}
        role="img"
        aria-label="Grafik nilai aset dari {day(first.date)} ({fmtIDR.format(first.total)}) sampai {day(last.date)} ({fmtIDR.format(last.total)})"
        onpointermove={move}
        onpointerleave={() => (hover = null)}
      >
        {#each geo.ticks as t}
          <line class="grid" x1={padL} x2={w - padR} y1={geo.y(t)} y2={geo.y(t)} />
          <text class="tick" x={w - padR + 6} y={geo.y(t) + 4}>{compact.format(t)}</text>
        {/each}
        <path class="area" d="{geo.total}L{geo.x(shown.length - 1)},{geo.base}L{geo.x(0)},{geo.base}Z" />
        <path class="l2" d={geo.invested} />
        <path class="l1" d={geo.total} />
        <text class="tick" x={padL} y={H - 6}>{day(first.date)}</text>
        <text class="tick" x={w - padR} y={H - 6} text-anchor="end">{day(last.date)}</text>
        {#if hp}
          <line class="cross" x1={geo.x(hover)} x2={geo.x(hover)} y1={padT} y2={H - padB} />
          <circle class="dot1" cx={geo.x(hover)} cy={geo.y(hp.total)} r="4" />
          <circle class="dot2" cx={geo.x(hover)} cy={geo.y(hp.invested)} r="4" />
        {/if}
      </svg>
      {#if hp}
        <div class="tip" style:left="{Math.min(geo.x(hover) + 12, w - 190)}px">
          <div class="tdate">{day(hp.date)}</div>
          <div><i class="key k1"></i><b>{fmtIDR.format(hp.total)}</b> Nilai</div>
          <div><i class="key k2"></i><b>{fmtIDR.format(hp.invested)}</b> Modal</div>
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .card {
    background: var(--surface);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 16px;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 16px;
    justify-content: space-between;
    align-items: flex-start;
  }
  h3 {
    margin: 0;
    font-size: 15px;
  }
  .sub {
    margin: 4px 0 0;
    font-size: 13px;
    color: var(--muted);
  }
  .pos {
    color: var(--in);
  }
  .neg {
    color: var(--out);
  }
  .ranges {
    display: flex;
    gap: 4px;
  }
  .ranges button {
    padding: 4px 10px;
    font-size: 13px;
    background: none;
  }
  .ranges .on {
    background: var(--bg);
    font-weight: 600;
  }
  .legend {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: var(--muted);
    margin: 12px 0 4px;
  }
  .key {
    display: inline-block;
    width: 14px;
    height: 0;
    border-top: 2px solid var(--series-1);
    vertical-align: middle;
    margin-right: 6px;
  }
  .k2 {
    border-top: 2px dashed var(--series-2);
  }
  .plot {
    position: relative;
    overflow: hidden;
    touch-action: pan-y;
  }
  svg {
    display: block;
    overflow: visible;
  }
  .grid {
    stroke: var(--line);
    stroke-width: 1;
  }
  .tick {
    fill: var(--muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .area {
    fill: var(--series-1);
    opacity: 0.08;
  }
  .l1,
  .l2 {
    fill: none;
    stroke-width: 2;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .l1 {
    stroke: var(--series-1);
  }
  .l2 {
    stroke: var(--series-2);
    stroke-dasharray: 5 4;
  }
  .cross {
    stroke: var(--muted);
    stroke-width: 1;
  }
  .dot1,
  .dot2 {
    stroke: var(--surface);
    stroke-width: 2;
  }
  .dot1 {
    fill: var(--series-1);
  }
  .dot2 {
    fill: var(--series-2);
  }
  .tip {
    position: absolute;
    top: 8px;
    pointer-events: none;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 8px 10px;
    font-size: 12px;
    color: var(--muted);
    display: grid;
    gap: 2px;
    min-width: 170px;
  }
  .tip b {
    color: var(--text);
    font-variant-numeric: tabular-nums;
    margin-right: 4px;
  }
  .tdate {
    color: var(--text);
  }
  .empty {
    color: var(--muted);
    font-size: 13px;
    margin: 16px 0 4px;
  }
</style>
