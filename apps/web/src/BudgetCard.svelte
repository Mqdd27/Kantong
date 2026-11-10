<script>
  import { s, fmtIDR } from './lib/store.svelte.js'

  /** compact: Home version with a link to the Budget page */
  let { compact = false } = $props()

  const b = $derived(s.budget)
  const limit = $derived(b?.overall ?? 0)
  const left = $derived(limit - (b?.spent ?? 0) - (b?.upcoming_total ?? 0))
  const pct = (v) => Math.min(100, (v / limit) * 100)

  // how far through the month today is (only for the current month)
  const elapsed = $derived.by(() => {
    if (!b?.days_left) return null
    const [y, m] = b.month.split('-').map(Number)
    const days = new Date(Date.UTC(y, m, 0)).getUTCDate()
    return ((days - b.days_left + 1) / days) * 100
  })
  const used = $derived(limit ? ((b.spent + b.upcoming_total) / limit) * 100 : 0)
  const state = $derived(left < 0 ? 'over' : used >= 90 || (elapsed != null && b.spent / limit * 100 > elapsed + 5) ? 'warn' : 'ok')
  const daily = $derived(b?.days_left && left > 0 ? left / b.days_left : 0)
</script>

<section class="card {state}">
  {#if !b}
    <p class="muted">Memuat budget…</p>
  {:else if !limit}
    <div class="row">
      <span class="muted">Belum ada budget bulanan.</span>
      <a class="btn" href="#/budget">Atur budget</a>
    </div>
  {:else}
    <div class="row">
      <span class="label">{left >= 0 ? 'Sisa bisa dibelanjakan' : 'Melebihi budget'}</span>
      {#if compact}<a class="link" href="#/budget">Detail ›</a>{/if}
    </div>
    <b class="big">{fmtIDR.format(Math.abs(left))}</b>
    <div class="track" role="img" aria-label="Terpakai {fmtIDR.format(b.spent)}, tagihan mendatang {fmtIDR.format(b.upcoming_total)}, dari budget {fmtIDR.format(limit)}">
      <span class="spent grow" style:width="{pct(b.spent)}%"></span>
      <span class="up grow" style:width="{Math.max(0, pct(b.spent + b.upcoming_total) - pct(b.spent))}%"></span>
      {#if elapsed != null}<i class="marker grow" style:left="{elapsed}%" title="Hari ini"></i>{/if}
    </div>
    <div class="legend">
      <span><i class="k spent"></i>Terpakai {fmtIDR.format(b.spent)}</span>
      {#if b.upcoming_total}<span><i class="k up"></i>Tagihan {fmtIDR.format(b.upcoming_total)}</span>{/if}
      <span>Budget {fmtIDR.format(limit)}</span>
    </div>
    {#if state === 'warn'}
      <p class="status">⚠︎ {used >= 90 ? `Sudah ${Math.round(used)}% terpakai` : 'Pengeluaran lebih cepat dari jadwal'}</p>
    {:else if state === 'over'}
      <p class="status">✕ Lewat budget {fmtIDR.format(-left)}</p>
    {/if}
    {#if daily}
      <p class="muted small">Jatah per hari <b>{fmtIDR.format(Math.floor(daily))}</b> · {b.days_left} hari lagi</p>
    {/if}
  {/if}
</section>

<style>
  .card {
    background: var(--surface);
    border-radius: 14px;
    padding: 16px;
    display: grid;
    gap: 8px;
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
  }
  .label,
  .muted {
    color: var(--muted);
    font-size: 13px;
    margin: 0;
  }
  .small {
    font-size: 13px;
  }
  .small b {
    color: var(--text);
  }
  .link {
    color: var(--muted);
    font-size: 13px;
    text-decoration: none;
  }
  .btn {
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 8px 14px;
    color: var(--text);
    text-decoration: none;
    font-size: 14px;
  }
  .big {
    font-size: 26px;
    font-variant-numeric: tabular-nums;
  }
  .over .big {
    color: var(--out);
  }
  .track {
    position: relative;
    display: flex;
    gap: 2px;
    height: 10px;
    border-radius: 5px;
    background: var(--bg);
  }
  .track > span {
    height: 100%;
    border-radius: 5px;
  }
  .spent {
    background: var(--accent);
  }
  .warn .spent {
    background: var(--warn);
  }
  .over .spent {
    background: var(--out);
  }
  .up {
    background: repeating-linear-gradient(135deg, var(--muted) 0 3px, transparent 3px 6px);
    opacity: 0.7;
  }
  .marker {
    position: absolute;
    top: -4px;
    bottom: -4px;
    width: 2px;
    margin-left: -1px;
    background: var(--text);
    border-radius: 1px;
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    font-size: 12px;
    color: var(--muted);
  }
  .k {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 3px;
    margin-right: 5px;
    vertical-align: -1px;
  }
  .status {
    margin: 0;
    font-size: 13px;
    font-weight: 600;
  }
  .warn .status {
    color: var(--warn);
  }
  .over .status {
    color: var(--out);
  }
</style>
