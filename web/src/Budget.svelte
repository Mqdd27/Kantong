<script>
  import { s, fmtIDR, fmtNum, fmtCur, setBudget, act } from './lib/store.svelte.js'
  import MonthBar from './MonthBar.svelte'
  import BudgetCard from './BudgetCard.svelte'
  import RecurringSheet from './RecurringSheet.svelte'

  const FREQ = { weekdays: 'hari kerja', weekly: 'mingguan', monthly: 'bulanan', yearly: 'tahunan' }
  const fmtDate = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', timeZone: 'UTC' })
  const dshort = (d) => fmtDate.format(new Date(d + 'T00:00:00Z'))
  const today = new Date(Date.now() + 7 * 3600e3).toISOString().slice(0, 10)

  // all expense categories, with this month's limit/spend when there is one
  const rows = $derived.by(() => {
    const by = new Map((s.budget?.categories ?? []).map((l) => [l.category, l]))
    return s.categories
      .filter((c) => c.type === 'out')
      .map((c) => by.get(c.name) ?? { category: c.name, limit: null, spent: 0, in_budget: c.name !== 'Investasi' })
      .sort((a, b) => (b.limit ?? -1) - (a.limit ?? -1) || b.spent - a.spent)
  })

  // edit mode: text inputs keyed by category ('' = overall)
  let editing = $state(false)
  let draft = $state({})
  function startEdit() {
    draft = { '': s.budget?.overall ? fmtNum.format(s.budget.overall) : '' }
    for (const r of rows) draft[r.category] = r.limit ? fmtNum.format(r.limit) : ''
    editing = true
  }
  const num = (v) => Number(String(v ?? '').replace(/\D/g, '')) || 0
  function fmtInput(e, k) {
    const n = num(e.currentTarget.value)
    draft[k] = n ? fmtNum.format(n) : ''
  }
  async function saveAll() {
    const old = { '': s.budget?.overall ?? 0 }
    for (const r of rows) old[r.category] = r.limit ?? 0
    for (const k of Object.keys(draft)) if (num(draft[k]) !== old[k]) await setBudget(k, num(draft[k]))
    editing = false
  }
  const catTotal = $derived(Object.entries(draft).reduce((n, [k, v]) => (k ? n + num(v) : n), 0))

  let sheet = $state(null) // null closed, {} new, {rec} edit

  // one row per recurring item: its first occurrence, plus how many more and their total
  const upcoming = $derived.by(() => {
    const by = new Map()
    for (const u of s.budget?.upcoming ?? []) {
      const g = by.get(u.recurring_id)
      if (g) (g.more++, (g.total += u.amount))
      else by.set(u.recurring_id, { ...u, more: 0, total: u.amount })
    }
    return [...by.values()]
  })
</script>

<MonthBar />

<div class="cols">
  <div>
    <BudgetCard />

    <div class="head">
      <h3>Per kategori</h3>
      {#if editing}
        <div class="btns">
          <button onclick={() => (editing = false)}>Batal</button>
          <button class="primary" onclick={saveAll}>Simpan</button>
        </div>
      {:else}
        <button onclick={startEdit}>Atur budget</button>
      {/if}
    </div>

    {#if editing}
      <p class="hint">Berlaku mulai bulan ini dan seterusnya sampai diubah. Kosongkan untuk menghapus.</p>
      <label class="edit overall">
        <span>Total bulanan</span>
        <input inputmode="numeric" placeholder="0" value={draft['']} oninput={(e) => fmtInput(e, '')} />
      </label>
      {#if num(draft['']) && catTotal > num(draft[''])}
        <p class="hint warn">⚠︎ Jumlah budget kategori ({fmtIDR.format(catTotal)}) lebih besar dari total.</p>
      {/if}
      {#each rows as r (r.category)}
        <label class="edit">
          <span>{r.category}{r.in_budget ? '' : ' (di luar total)'}</span>
          <input inputmode="numeric" placeholder="tanpa batas" value={draft[r.category]} oninput={(e) => fmtInput(e, r.category)} />
        </label>
      {/each}
    {:else}
      <ul class="cats">
        {#each rows.filter((r) => r.limit || r.spent) as r (r.category)}
          {@const p = r.limit ? (r.spent / r.limit) * 100 : 0}
          <li>
            <div class="line">
              <span>{r.category}{r.in_budget ? '' : ' ·  di luar total'}</span>
              <span class="amt">
                {fmtIDR.format(r.spent)}{#if r.limit}<span class="muted"> / {fmtIDR.format(r.limit)}</span>{/if}
              </span>
            </div>
            {#if r.limit}
              <div class="bar"><span class:warn={p >= 90 && p <= 100} class:over={p > 100} style:width="{Math.min(p, 100)}%"></span></div>
              <div class="sub">
                {#if p > 100}<span class="overtxt">✕ lewat {fmtIDR.format(r.spent - r.limit)}</span>
                {:else}sisa {fmtIDR.format(r.limit - r.spent)}{/if}
              </div>
            {/if}
          </li>
        {:else}
          <li class="muted">Belum ada pengeluaran bulan ini.</li>
        {/each}
      </ul>
    {/if}
  </div>

  <div>
    <div class="head">
      <h3>Akan datang bulan ini</h3>
    </div>
    {#each upcoming as u (u.recurring_id)}
      {@const due = u.date <= today}
      <div class="up">
        <span class="main">
          <span>{u.merchant || u.category}</span>
          <span class="muted">{dshort(u.date)}{due ? ' · jatuh tempo' : ''}{u.more ? ` · +${u.more}× lagi, total ${fmtIDR.format(u.total)}` : ''}{u.auto ? ' · otomatis' : ''}</span>
        </span>
        <span class="amt" class:in={u.type === 'in'}>{u.type === 'in' ? '+' : ''}{fmtIDR.format(u.amount)}</span>
        {#if due && !u.auto && s.recurring.find((r) => r.id === u.recurring_id)?.next_date === u.date}
          <span class="btns">
            <button class="primary sm" onclick={() => act(`/recurring/${u.recurring_id}/advance`, 'POST')}>Catat</button>
            <button class="sm" onclick={() => act(`/recurring/${u.recurring_id}/advance?skip=1`, 'POST')}>Lewati</button>
          </span>
        {/if}
      </div>
    {:else}
      <p class="muted">Nggak ada tagihan lagi bulan ini.</p>
    {/each}

    <div class="head">
      <h3>Transaksi berulang</h3>
      <button onclick={() => (sheet = {})}>+ Tambah</button>
    </div>
    {#each s.recurring as r (r.id)}
      <button class="rec" onclick={() => (sheet = { rec: r })}>
        <span class="main">
          <span>{r.merchant || r.category}</span>
          <span class="muted">{r.category} · {FREQ[r.freq]} · berikutnya {dshort(r.next_date)}{r.auto ? ' · otomatis' : ''}</span>
        </span>
        <span class="amt" class:in={r.type === 'in'}>
          {r.currency !== 'IDR' ? fmtCur(r.orig_amount, r.currency) : fmtIDR.format(r.amount)}
        </span>
      </button>
    {:else}
      <p class="muted">Tambahkan gaji, langganan (Claude, internet), cicilan, dll. Tagihan yang akan datang ikut mengurangi sisa budget.</p>
    {/each}
  </div>
</div>

{#if sheet}
  <RecurringSheet rec={sheet.rec} onclose={() => (sheet = null)} />
{/if}

<style>
  .cols {
    display: grid;
    gap: 8px;
  }
  @media (min-width: 900px) {
    .cols {
      grid-template-columns: 1fr 1fr;
      gap: 40px;
      align-items: start;
    }
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin: 24px 0 8px;
    gap: 8px;
  }
  .cols > div > .head:first-child {
    margin-top: 0;
  }
  @media (max-width: 899px) {
    .cols > div > .head:first-child {
      margin-top: 24px;
    }
  }
  h3 {
    font-size: 14px;
    color: var(--muted);
    margin: 0;
  }
  .head button {
    padding: 6px 12px;
    font-size: 14px;
  }
  .btns {
    display: flex;
    gap: 6px;
  }
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .hint {
    color: var(--muted);
    font-size: 13px;
    margin: 0 0 8px;
  }
  .warn {
    color: var(--warn);
  }
  .edit {
    display: grid;
    grid-template-columns: 1fr 160px;
    align-items: center;
    gap: 12px;
    padding: 6px 0;
    font-size: 14px;
  }
  .edit.overall {
    font-weight: 600;
    border-bottom: 1px solid var(--line);
    padding-bottom: 12px;
    margin-bottom: 4px;
  }
  .edit input {
    text-align: right;
    padding: 8px 10px;
  }
  .cats {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 14px;
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
  .in {
    color: var(--in);
  }
  .bar {
    height: 8px;
    border-radius: 4px;
    background: var(--surface);
    margin: 6px 0 2px;
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: 4px;
    background: var(--accent);
  }
  .bar .warn {
    background: var(--warn);
  }
  .bar .over {
    background: var(--out);
  }
  .sub {
    font-size: 12px;
    color: var(--muted);
  }
  .overtxt {
    color: var(--out);
    font-weight: 600;
  }
  .up,
  .rec {
    display: flex;
    flex-wrap: wrap;
    width: 100%;
    gap: 6px 12px;
    align-items: center;
    padding: 12px 0;
    border: 0;
    border-bottom: 1px solid var(--line);
    border-radius: 0;
    background: none;
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
  .sm {
    padding: 4px 10px;
    font-size: 13px;
  }
</style>
