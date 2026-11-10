<script>
  import { onMount } from 'svelte'
  import PerfChart from './PerfChart.svelte'
  import TxRow from './TxRow.svelte'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'
  import { kindLook } from './lib/look.js'
  import { api } from './lib/api.js'
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
      ? { ...a, invested: a.invested ?? '', target: a.target ?? '', target_date: a.target_date ?? '', return_pct: a.return_pct ?? '', maturity: a.maturity ?? '', face: a.face ?? '' }
      : { name: '', kind: 'bank', currency: 'IDR', balance: '', invested: '', note: '', target: '', target_date: '', return_pct: '', maturity: '', payout: 0, face: '' }
    edit = a ?? {}
    // cash-like accounts open on their history, investments on the edit form
    tab = a && CASHLIKE.has(a.kind) ? 'history' : 'edit'
    act = null
    if (a) api(`/assets/${a.id}/activity`).then((r) => (act = r)).catch(() => (act = { transactions: [], orders: [] }))
  }
  const CASHLIKE = new Set(['bank', 'ewallet', 'forex', 'cash'])
  const STATUS = { pending: 'menunggu', success: 'berhasil', failed: 'gagal' }
  let tab = $state('edit')
  let act = $state(null)
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
  const pctFmt = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 3 })
  // expected return: net coupon per payout (bonds that pay), value at maturity (bonds that don't), or projected value
  function proj(a) {
    const r = a.return_pct / 100, pct = `Return ${pctFmt.format(a.return_pct)}%/thn`
    const end = a.maturity && fmtMon.format(new Date(a.maturity + 'T00:00:00Z'))
    if (a.kind === 'bank') {
      const month = (a.balance * r * (1 - BANK_TAX)) / 12
      return `Bunga ${pctFmt.format(a.return_pct)}%/thn ≈ ${fmtCur(month, a.currency)}/bln bersih pajak · 1 thn ≈ ${fmtCur(month * 12, a.currency)}`
    }
    if (BONDS.has(a.kind)) {
      // coupons on the nominal + repayment of the nominal at maturity (gain vs modal), both net of tax
      const face = a.face ?? a.invested ?? a.balance
      const pull = a.maturity && a.invested != null ? (face - a.invested) * (a.face ? 1 - TAX : 1) : 0
      if (a.payout) {
        const per = (face * r * a.payout * (1 - TAX)) / 12
        let n = 0 // coupon dates left: step back from maturity by the payout period
        for (const d = new Date(a.maturity + 'T00:00:00Z'); a.maturity && d > new Date(); d.setUTCMonth(d.getUTCMonth() - a.payout)) n++
        let txt = `${pct} · kupon ≈ ${fmtCur(per, a.currency)} tiap ${PAYOUT[a.payout]} (bersih pajak)`
        if (n) txt += ` · ${n}× lagi s.d. ${end}` + (pull ? `, + selisih pelunasan ${fmtCur(pull, a.currency)}` : '') + ` · total return ≈ ${fmtCur(per * n + pull, a.currency)}`
        return txt
      }
      if (a.maturity) {
        const gain = a.balance * ((1 + r) ** (monthsTo(a.maturity) / 12) - 1) * (1 - TAX)
        return `${pct}, tanpa kupon · di jatuh tempo ${end} ≈ ${fmtCur(a.balance + gain, a.currency)} (bersih pajak)`
      }
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
      payout: BONDS.has(f.kind) ? Number(f.payout) || 0 : 0,
      face: BONDS.has(f.kind) && Number(f.face) > 0 ? Number(f.face) : null }
    if (await saveAsset(edit.id, body)) dialog.close()
  }
  async function del() {
    await deleteAsset(edit.id)
    dialog.close()
  }
</script>

<PageHead title="Aset">
  <button class="icon" aria-label="Tambah aset" title="Tambah aset" onclick={() => openEdit()}><Icon name="plus" /></button>
</PageHead>

<div class="top">
<section class="card hero">
  <span class="head">Total aset</span>
  <b class="num">{fmtIDR.format(s.portfolio.total_idr)}</b>
  {#if s.portfolio.total_idr > 0}
    <ul class="alloc" aria-label="Alokasi per jenis">
      {#each groups.filter((g) => g.total > 0) as g}
        {@const pct = (g.total / s.portfolio.total_idr) * 100}
        <li>
          <span>{KINDS[g.kind]}</span>
          <span class="bar"><span class="grow" style:width="{pct}%" style:background={kindLook(g.kind).hue}></span></span>
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
  <div class="sec"><h2>Order menunggu konfirmasi</h2></div>
  <section class="card list pending">
  {#each s.portfolio.pending_orders as o}
    <div class="row">
      <span class="avatar" style:--hue="var(--warn)"><Icon name={o.side === 'buy' ? 'down' : 'up'} /></span>
      <span class="main"><span class="name">{o.side === 'buy' ? 'Beli' : 'Jual'} {o.asset}</span>
        <span class="meta">{o.order_ref} · {fmtShort.format(new Date(o.occurred_at))}</span></span>
      <span class="num">{fmtIDR.format(o.amount)}</span>
    </div>
  {/each}
  </section>
{/if}

<div class="groups">
{#each groups as g}
  {@const look = kindLook(g.kind)}
  <section class="card list group">
  <h3><span>{KINDS[g.kind]}</span><span class="num">{fmtIDR.format(g.total)}</span></h3>
  {#each g.items as a}
    <button class="row" onclick={() => openEdit(a)}>
      <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
      <span class="main">
        <span class="name">{a.name}</span>
        <span class="meta">
          {#if a.currency !== 'IDR'}{fmtCur(a.balance, a.currency)} · 1 {a.currency === 'XAU' ? 'gram' : a.currency} = {fmtIDR.format(a.rate)}{/if}
          {#if a.invested}{a.currency !== 'IDR' ? ' · ' : ''}modal {fmtCur(a.invested, a.currency === 'XAU' ? 'IDR' : a.currency)}{/if}
        </span>
        {#if a.return_pct != null && RETURNS.has(a.kind)}<span class="meta proj">{proj(a)}</span>{/if}
        {#if a.target}
          {@const g = goal(a)}
          <span class="goal">
            <span class="gbar"><span style:width="{Math.min(100, g.pct)}%"></span></span>
            <span class="meta">
              {g.pct >= 100 ? 'Target tercapai' : `${Math.floor(g.pct)}% dari ${fmtCur(a.target, a.currency)}`}{#if g.perMonth}
                · perlu {fmtCur(g.perMonth, a.currency)}/bln sampai {g.by}{/if}
            </span>
          </span>
        {/if}
      </span>
      <span class="val">
        {fmtIDR.format(a.value_idr)}
        {#if a.invested}
          {@const pl = ((a.currency === 'XAU' ? a.value_idr : a.balance) - a.invested) / a.invested}
          <small class:in={pl >= 0} class:out={pl < 0}>{pl >= 0 ? '+' : ''}{(pl * 100).toFixed(1)}%</small>
        {/if}
      </span>
    </button>
  {/each}
  </section>
{:else}
  <p class="card empty">Belum ada aset. Tambah rekening, valas, saham (total nilai), obligasi, SBN, dll.</p>
{/each}
</div>

<button class="add" onclick={() => openEdit()}><Icon name="plus" size={18} />Tambah aset</button>

{#if edit}
  <dialog class="sheet" bind:this={dialog} onclose={() => (edit = null)} onclick={(e) => e.target === dialog && dialog.close()}>
    {#if edit.id}
      <div class="tabs seg">
        <button type="button" aria-pressed={tab === 'history'} onclick={() => (tab = 'history')}>Riwayat</button>
        <button type="button" aria-pressed={tab === 'edit'} onclick={() => (tab = 'edit')}>Edit</button>
      </div>
    {/if}
    {#if tab === 'history'}
      <section class="hist">
        <h2>{edit.name}</h2>
        <p class="muted small">Transaksi yang pakai akun “{edit.name}”, termasuk transfer masuk dan penyesuaian saldo.</p>
        {#if !act}
          <p class="muted">Memuat…</p>
        {:else}
          {#each act.orders as o (o.id)}
            <div class="order">
              <span>{o.side === 'buy' ? 'Beli' : 'Jual'} · {STATUS[o.status]}<small>{fmtShort.format(new Date(o.occurred_at))}</small></span>
              <b>{fmtCur(o.amount, edit.currency)}</b>
            </div>
          {/each}
          {#each act.transactions as tx (tx.id)}
            <TxRow {tx} showDate account={edit.name} />
          {/each}
          {#if !act.transactions.length && !act.orders.length}
            <p class="muted">Belum ada transaksi dengan akun “{edit.name}”. Isi kolom Akun saat mencatat transaksi.</p>
          {/if}
        {/if}
        <div class="actions"><button type="button" onclick={() => dialog.close()}>Tutup</button></div>
      </section>
    {:else}
    <form onsubmit={save}>
      <h2>{edit.id ? 'Edit aset' : 'Tambah aset'}</h2>
      <label>Nama <input bind:value={f.name} required placeholder="mis. Jago Pocket SGD, Stockbit, ORI027" /></label>
      <div class="two">
        <label>Jenis
          <select bind:value={f.kind} onchange={() => { if (f.kind === 'gold' && f.currency === 'IDR') f.currency = 'XAU' }}>{#each Object.entries(KINDS) as [k, v]}<option value={k}>{v}</option>{/each}</select>
        </label>
        <label>Mata uang
          <select bind:value={f.currency}>{#each CURRENCIES as c}<option value={c}>{c === 'XAU' ? 'Emas (gram)' : c}</option>{/each}</select>
        </label>
      </div>
      {#if f.kind === 'gold' && f.currency !== 'XAU'}
        <small class="hint">Pilih mata uang <b>Emas (gram)</b> lalu isi jumlah gram, biar nilainya ikut harga emas harian.</small>
      {/if}
      <label>{f.currency === 'XAU' ? 'Jumlah (gram)' : `Nilai sekarang (${f.currency})`}
        <input type="number" inputmode="decimal" step="any" bind:value={f.balance} required />
      </label>
      <label>Modal / total disetor ({f.currency === 'XAU' ? 'IDR' : f.currency}), opsional
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
        <div class="two">
          <label>Nominal ({f.currency})
            <input type="number" inputmode="decimal" step="any" min="0" bind:value={f.face} placeholder="mis. 5000000" />
          </label>
          <label>Jatuh tempo <input type="date" bind:value={f.maturity} /></label>
        </div>
        <small class="hint">Return = kupon/tahun (bukan yield). Nominal = jumlah unit × Rp1.000.000; kupon dihitung dari nominal, dipotong pajak 10%. Contoh kupon: ORI/SR/ST tiap bulan, FR/PBS tiap 6 bulan.</small>
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
          <label>Target tabungan ({f.currency === 'XAU' ? 'gram' : f.currency}), opsional
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
    {/if}
  </dialog>
{/if}

<style>
  .top {
    display: grid;
    gap: 12px;
    margin-bottom: 12px;
  }
  .top > :global(*) {
    min-width: 0; /* let the chart shrink with the column */
  }
  .groups {
    display: grid;
    gap: 12px;
    margin-top: 12px;
  }
  @media (min-width: 900px) {
    .top {
      grid-template-columns: minmax(260px, 1fr) 2.2fr;
      gap: 24px;
      align-items: start;
    }
    .groups {
      grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
      gap: 24px;
      align-items: start;
    }
    .add {
      max-width: 340px;
    }
  }
  .hero {
    display: grid;
    gap: 4px;
  }
  .head {
    color: var(--muted);
    font-size: 13px;
  }
  .hero b {
    font-size: 30px;
    font-weight: 700;
    letter-spacing: -0.01em;
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
    background: var(--bg);
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
    gap: 8px;
    margin: 0;
    padding: 12px 0 8px;
    border-bottom: 1px solid var(--line);
    font-size: 13px;
    font-weight: 600;
    color: var(--muted);
  }
  h3 .num {
    color: var(--text);
  }
  .pending {
    margin-bottom: 4px;
  }
  .name {
    font-weight: 600;
  }
  .row:last-child {
    border-bottom: 0;
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
    border-radius: 0;
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
    font-weight: 650;
    justify-items: end;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  small {
    font-size: 12px;
    font-weight: 500;
  }
  .in {
    color: var(--in);
  }
  .out {
    color: var(--out);
  }
  .add {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    width: 100%;
    margin-top: 12px;
    padding: 12px;
    border: 1px dashed var(--field);
    border-radius: var(--radius);
    background: none;
    color: var(--accent);
    font-weight: 600;
  }
  .tabs {
    padding: 16px 16px 0;
  }
  .hist {
    padding: 16px 16px calc(env(safe-area-inset-bottom) + 16px);
    max-height: 75vh;
    overflow-y: auto;
  }
  .hist .muted {
    color: var(--muted);
  }
  .hist .small {
    font-size: 13px;
    margin: 4px 0 8px;
  }
  .order {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 0;
    border-bottom: 1px solid var(--line);
  }
  .order span {
    display: grid;
  }
  .order small {
    color: var(--muted);
    font-size: 13px;
  }
  button.row:active {
    scale: none;
    background: color-mix(in srgb, var(--text) 6%, transparent);
  }
</style>
