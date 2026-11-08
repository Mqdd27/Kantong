<script>
  import { onMount } from 'svelte'
  import PerfChart from './PerfChart.svelte'
  import { s, fmtIDR, fmtCur, fmtShort, loadAssets, CURRENCIES, saveAsset, deleteAsset } from './lib/store.svelte.js'

  const KINDS = {
    bank: 'Bank', ewallet: 'E-wallet', forex: 'Valas', cash: 'Tunai', stock: 'Saham', bond: 'Obligasi',
    sbn: 'SBN', mutual_fund: 'Reksa dana', gold: 'Emas', crypto: 'Kripto', other: 'Lainnya',
  }

  const BONDS = new Set(['bond', 'sbn']) // payout + maturity instead of a savings goal
  const PAYOUT = { 1: 'bulan', 3: '3 bulan', 6: '6 bulan', 12: 'tahun' } // months between coupons; 0 = none
  const RETURNS = new Set(['bond', 'sbn', 'mutual_fund', 'bank']) // only these get a return projection
  const TAX = 0.1 // final tax on bond/SBN coupons for individuals
  const BANK_TAX = 0.2 // final tax on savings/deposit interest

  onMount(loadAssets)

  const groups = $derived.by(() => {
    const m = new Map()
    for (const a of s.portfolio.assets) {
      const g = m.get(a.kind) ?? { kind: a.kind, total: 0, items: [] }
      g.total += a.value_idr || 0
      g.items.push(a)
      m.set(a.kind, g)
    }
    return [...m.values()].sort((a, b) => b.total - a.total)
  })

  // edit sheet: null = closed, {} = new, asset = edit
  let edit = $state(null)
  let dialog = $state()
  let f = $state({})
  $effect(() => {
    if (edit && dialog && !dialog.open) dialog.showModal()
  })
  function openEdit(a) {
    f = a
      ? { ...a, invested: a.invested ?? '', target: a.target ?? '', target_date: a.target_date ?? '', return_pct: a.return_pct ?? '', maturity: a.maturity ?? '' }
      : { name: '', kind: 'bank', currency: 'IDR', balance: '', invested: '', note: '', target: '', target_date: '', return_pct: '', maturity: '', payout: 0 }
    edit = a ?? {}
  }
  const fmtMon = new Intl.DateTimeFormat('id-ID', { month: 'short', year: 'numeric', timeZone: 'UTC' })
  // savings goal: progress and the monthly amount still needed to hit it by target_date
  function goal(a) {
    const pct = (a.balance / a.target) * 100
    if (pct >= 100 || !a.target_date) return { pct }
    const t = new Date(a.target_date + 'T00:00:00Z'), now = new Date()
    const months = Math.max(1, (t.getUTCFullYear() - now.getUTCFullYear()) * 12 + t.getUTCMonth() - now.getUTCMonth())
    return { pct, perMonth: (a.target - a.balance) / months, by: fmtMon.format(t) }
  }

  const monthsTo = (d) => {
    const t = new Date(d + 'T00:00:00Z'), now = new Date()
    return Math.max(0, (t.getUTCFullYear() - now.getUTCFullYear()) * 12 + t.getUTCMonth() - now.getUTCMonth())
  }
  const pctFmt = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 2 })
  // expected return: net coupon per payout (bonds that pay), value at maturity (bonds that don't), or projected value
  function proj(a) {
    const r = a.return_pct / 100, pct = `Return ${pctFmt.format(a.return_pct)}%/thn`
    const end = a.maturity && fmtMon.format(new Date(a.maturity + 'T00:00:00Z'))
    if (a.kind === 'bank') {
      const month = (a.balance * r * (1 - BANK_TAX)) / 12
      return `Bunga ${pctFmt.format(a.return_pct)}%/thn ≈ ${fmtCur(month, a.currency)}/bln bersih pajak · 1 thn ≈ ${fmtCur(month * 12, a.currency)}`
    }
    if (BONDS.has(a.kind) && a.payout) {
      const per = ((a.invested ?? a.balance) * r * a.payout * (1 - TAX)) / 12
      let n = 0 // coupon dates left: step back from maturity by the payout period
      for (const d = new Date(a.maturity + 'T00:00:00Z'); a.maturity && d > new Date(); d.setUTCMonth(d.getUTCMonth() - a.payout)) n++
      return `${pct} · kupon ≈ ${fmtCur(per, a.currency)} tiap ${PAYOUT[a.payout]} (bersih pajak)` +
        (n ? ` · ${n}× lagi s.d. ${end}, total ≈ ${fmtCur(per * n, a.currency)}` : '')
    }
    if (BONDS.has(a.kind) && a.maturity) {
      const gain = a.balance * ((1 + r) ** (monthsTo(a.maturity) / 12) - 1) * (1 - TAX)
      return `${pct}, tanpa kupon · di jatuh tempo ${end} ≈ ${fmtCur(a.balance + gain, a.currency)} (bersih pajak)`
    }
    let txt = `${pct} · 1 thn ≈ ${fmtCur(a.balance * (1 + r), a.currency)}`
    if (a.target_date) txt += ` · ${fmtMon.format(new Date(a.target_date + 'T00:00:00Z'))} ≈ ${fmtCur(a.balance * (1 + r) ** (monthsTo(a.target_date) / 12), a.currency)}`
    return txt
  }

  async function save(e) {
    e.preventDefault()
    const body = { name: f.name, kind: f.kind, currency: f.currency, balance: Number(f.balance) || 0, note: f.note,
      invested: f.invested === '' || f.invested == null ? null : Number(f.invested),
      target: !BONDS.has(f.kind) && Number(f.target) > 0 ? Number(f.target) : null,
      target_date: !BONDS.has(f.kind) && f.target ? f.target_date || null : null,
      return_pct: !RETURNS.has(f.kind) || f.return_pct === '' || f.return_pct == null ? null : Number(f.return_pct),
      maturity: BONDS.has(f.kind) ? f.maturity || null : null,
      payout: BONDS.has(f.kind) ? Number(f.payout) || 0 : 0 }
    if (await saveAsset(edit.id, body)) dialog.close()
  }
  async function del() {
    await deleteAsset(edit.id)
    dialog.close()
  }
</script>

<div class="top">
<section class="hero">
  <span>Total aset</span>
  <b>{fmtIDR.format(s.portfolio.total_idr)}</b>
  {#if s.portfolio.total_idr > 0}
    <ul class="alloc" aria-label="Alokasi per jenis">
      {#each groups.filter((g) => g.total > 0) as g}
        {@const pct = (g.total / s.portfolio.total_idr) * 100}
        <li>
          <span>{KINDS[g.kind]}</span>
          <span class="bar"><span style:width="{pct}%"></span></span>
          <span class="pct">{pct.toFixed(1)}%</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#if s.portfolio.missing_rates.length}
    <small class="warn">Kurs belum tersedia: {s.portfolio.missing_rates.join(', ')} (tidak dihitung)</small>
  {/if}
</section>

<PerfChart points={s.history} />
</div>

{#if s.portfolio.pending_orders.length}
  <h3>Order menunggu konfirmasi</h3>
  {#each s.portfolio.pending_orders as o}
    <div class="row">
      <span class="main"><span>{o.side === 'buy' ? 'Beli' : 'Jual'} {o.asset}</span>
        <span class="meta">{o.order_ref} · {fmtShort.format(new Date(o.occurred_at))}</span></span>
      <span>{fmtIDR.format(o.amount)}</span>
    </div>
  {/each}
{/if}

<div class="groups">
{#each groups as g}
  <div class="group">
  <h3><span>{KINDS[g.kind]}</span><span>{fmtIDR.format(g.total)}</span></h3>
  {#each g.items as a}
    <button class="row" onclick={() => openEdit(a)}>
      <span class="main">
        <span>{a.name}</span>
        <span class="meta">
          {#if a.currency !== 'IDR'}{fmtCur(a.balance, a.currency)} · 1 {a.currency} = {fmtIDR.format(a.rate)}{/if}
          {#if a.invested}{a.currency !== 'IDR' ? ' · ' : ''}modal {fmtCur(a.invested, a.currency)}{/if}
        </span>
        {#if a.return_pct != null && RETURNS.has(a.kind)}<span class="meta proj">{proj(a)}</span>{/if}
        {#if a.target}
          {@const g = goal(a)}
          <span class="goal">
            <span class="gbar"><span style:width="{Math.min(100, g.pct)}%"></span></span>
            <span class="meta">
              {g.pct >= 100 ? '✓ Target tercapai' : `${Math.floor(g.pct)}% dari ${fmtCur(a.target, a.currency)}`}{#if g.perMonth}
                · perlu {fmtCur(g.perMonth, a.currency)}/bln sampai {g.by}{/if}
            </span>
          </span>
        {/if}
      </span>
      <span class="val">
        {fmtIDR.format(a.value_idr)}
        {#if a.invested}
          {@const pl = (a.balance - a.invested) / a.invested}
          <small class:in={pl >= 0} class:out={pl < 0}>{pl >= 0 ? '+' : ''}{(pl * 100).toFixed(1)}%</small>
        {/if}
      </span>
    </button>
  {/each}
  </div>
{:else}
  <p class="empty">Belum ada aset. Tambah rekening, valas, saham (total nilai), obligasi, SBN, dll.</p>
{/each}
</div>

<button class="add" onclick={() => openEdit()}>+ Tambah aset</button>

{#if edit}
  <dialog bind:this={dialog} onclose={() => (edit = null)} onclick={(e) => e.target === dialog && dialog.close()}>
    <form onsubmit={save}>
      <h2>{edit.id ? 'Edit aset' : 'Tambah aset'}</h2>
      <label>Nama <input bind:value={f.name} required placeholder="mis. Jago Pocket SGD, Stockbit, ORI027" /></label>
      <div class="two">
        <label>Jenis
          <select bind:value={f.kind}>{#each Object.entries(KINDS) as [k, v]}<option value={k}>{v}</option>{/each}</select>
        </label>
        <label>Mata uang
          <select bind:value={f.currency}>{#each CURRENCIES as c}<option>{c}</option>{/each}</select>
        </label>
      </div>
      <label>Nilai sekarang ({f.currency})
        <input type="number" inputmode="decimal" step="any" bind:value={f.balance} required />
      </label>
      <label>Modal / total disetor ({f.currency}), opsional
        <input type="number" inputmode="decimal" step="any" bind:value={f.invested} />
      </label>
      {#if BONDS.has(f.kind)}
        <div class="two">
          <label>Return (% per tahun)
            <input type="number" inputmode="decimal" step="any" bind:value={f.return_pct} placeholder="mis. 6,25" />
          </label>
          <label>Kupon dibayar
            <select bind:value={f.payout}>
              <option value={1}>Tiap bulan</option>
              <option value={3}>Tiap 3 bulan</option>
              <option value={6}>Tiap 6 bulan</option>
              <option value={12}>Tiap tahun</option>
              <option value={0}>Tidak ada kupon</option>
            </select>
          </label>
        </div>
        <label>Jatuh tempo <input type="date" bind:value={f.maturity} /></label>
        <small class="hint">Contoh: SBN ritel (ORI/SR/ST) tiap bulan, FR/PBS tiap 6 bulan. Dihitung dari modal, dipotong pajak 10%.</small>
      {:else}
        {#if RETURNS.has(f.kind)}
          {#if f.kind === 'bank'}
            <label>Bunga (% per tahun), opsional
              <input type="number" inputmode="decimal" step="any" bind:value={f.return_pct} placeholder="mis. 3" />
            </label>
            <small class="hint">Dihitung dari nilai sekarang, dipotong pajak bunga 20%.</small>
          {:else}
            <label>Proyeksi return (% per tahun), opsional
              <input type="number" inputmode="decimal" step="any" bind:value={f.return_pct} placeholder="mis. 4,5 untuk RDPU" />
            </label>
          {/if}
        {/if}
        <div class="two">
          <label>Target tabungan ({f.currency}), opsional
            <input type="number" inputmode="decimal" step="any" min="0" bind:value={f.target} />
          </label>
          <label>Tercapai pada
            <input type="date" bind:value={f.target_date} />
          </label>
        </div>
      {/if}
      <label>Catatan <input bind:value={f.note} /></label>
      <div class="actions">
        {#if edit.id}<button type="button" class="danger" onclick={del}>Hapus</button>{/if}
        <button type="button" onclick={() => dialog.close()}>Batal</button>
        <button class="primary">Simpan</button>
      </div>
    </form>
  </dialog>
{/if}

<style>
  .top {
    display: grid;
    gap: 8px;
  }
  .top > :global(*) {
    min-width: 0; /* let the chart shrink with the column */
  }
  .groups {
    display: grid;
    gap: 0 32px;
  }
  @media (min-width: 900px) {
    .top {
      grid-template-columns: minmax(240px, 1fr) 2.4fr;
      gap: 16px;
      align-items: start;
    }
    .groups {
      grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    }
    .add {
      max-width: 340px;
    }
  }
  .hero {
    display: grid;
    gap: 4px;
    padding: 16px;
    background: var(--surface);
    border-radius: 14px;
    margin-bottom: 8px;
  }
  .hero > span {
    color: var(--muted);
    font-size: 13px;
  }
  .hero b {
    font-size: 28px;
    font-variant-numeric: tabular-nums;
  }
  .alloc {
    list-style: none;
    padding: 0;
    margin: 12px 0 0;
    display: grid;
    gap: 8px;
  }
  .alloc li {
    display: grid;
    grid-template-columns: 84px 1fr 48px;
    gap: 8px;
    align-items: center;
    font-size: 13px;
  }
  .alloc .bar {
    height: 6px;
    border-radius: 3px;
    background: var(--bg);
    overflow: hidden;
  }
  .alloc .bar span {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--series-1);
  }
  .alloc .pct {
    text-align: right;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .proj {
    color: var(--accent);
  }
  .hint {
    margin-top: -6px;
    font-size: 12px;
    color: var(--muted);
  }
  .goal {
    display: grid;
    gap: 3px;
    margin-top: 4px;
  }
  .gbar {
    height: 6px;
    border-radius: 3px;
    background: var(--surface);
    overflow: hidden;
    max-width: 240px;
  }
  .gbar span {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--in);
  }
  .warn {
    color: var(--warn);
  }
  h3 {
    display: flex;
    justify-content: space-between;
    font-size: 14px;
    color: var(--muted);
    margin: 20px 0 4px;
  }
  .row {
    display: flex;
    width: 100%;
    gap: 12px;
    align-items: center;
    padding: 12px 0;
    background: none;
    border: 0;
    border-bottom: 1px solid var(--line);
    color: inherit;
    text-align: left;
    font: inherit;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
  }
  .meta {
    color: var(--muted);
    font-size: 13px;
  }
  .val {
    display: grid;
    justify-items: end;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  small {
    font-size: 12px;
  }
  .in {
    color: var(--in);
  }
  .out {
    color: var(--out);
  }
  .empty {
    color: var(--muted);
    text-align: center;
    padding: 24px 0;
  }
  .add {
    width: 100%;
    margin-top: 16px;
    padding: 12px;
    border: 1px dashed var(--line);
    border-radius: 10px;
    background: none;
    color: var(--text);
  }
  dialog {
    border: 0;
    padding: 0;
    width: 100%;
    max-width: 640px;
    margin: auto auto 0;
    border-radius: 16px 16px 0 0;
    background: var(--bg);
    color: var(--text);
  }
  @media (min-width: 900px) {
    dialog {
      max-width: 480px;
      margin: auto;
      border-radius: 16px;
    }
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.45);
  }
  form {
    display: grid;
    gap: 12px;
    padding: 20px 16px calc(env(safe-area-inset-bottom) + 16px);
  }
  h2 {
    margin: 0;
    font-size: 18px;
  }
  label {
    display: grid;
    gap: 4px;
    font-size: 13px;
    color: var(--muted);
  }
  .two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .danger {
    margin-right: auto;
    color: var(--out);
  }
</style>
