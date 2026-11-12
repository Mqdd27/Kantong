<script>
  import { s, fmtIDR, fmtNum, fmtCur, setBudget, act, freqLabel } from './lib/store.svelte.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import Icon from './Icon.svelte'
  import { catLook } from './lib/look.js'
  import BudgetCard from './BudgetCard.svelte'
  import RecurringSheet from './RecurringSheet.svelte'

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

<PageHead title="Budget" />
<MonthBar />

<div class="cols">
  <div class="stack">
    <BudgetCard />

    <div class="sec">
      <h2>Per kategori</h2>
      {#if editing}
        <span class="btns">
          <button onclick={() => (editing = false)}>Batal</button>
          <button class="primary" onclick={saveAll}>Simpan</button>
        </span>
      {:else}
        <button onclick={startEdit}>Atur budget</button>
      {/if}
    </div>

    {#if editing}
      <section class="card">
        <p class="hint">Berlaku mulai bulan ini dan seterusnya sampai diubah. Kosongkan untuk menghapus.</p>
        <label class="edit overall">
          <span>Total bulanan</span>
          <input inputmode="numeric" placeholder="0" value={draft['']} oninput={(e) => fmtInput(e, '')} />
        </label>
        {#if num(draft['']) && catTotal > num(draft[''])}
          <p class="hint warn"><Icon name="alert" size={16} />Jumlah budget kategori ({fmtIDR.format(catTotal)}) lebih besar dari total.</p>
        {/if}
        {#each rows as r (r.category)}
          {@const look = catLook(r.category)}
          <label class="edit">
            <span class="name"><span class="avatar sm" style:--hue={look.hue}><Icon name={look.icon} size={18} /></span>{r.category}{r.in_budget ? '' : ' (di luar total)'}</span>
            <input inputmode="numeric" placeholder="tanpa batas" value={draft[r.category]} oninput={(e) => fmtInput(e, r.category)} />
          </label>
        {/each}
      </section>
    {:else}
      <section class="card">
        <ul class="cats">
          {#each rows.filter((r) => r.limit || r.spent) as r (r.category)}
            {@const p = r.limit ? (r.spent / r.limit) * 100 : 0}
            {@const look = catLook(r.category)}
            <li>
              <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
              <div class="what">
                <div class="line">
                  <span class="cat">{r.category}{#if !r.in_budget}<small> · di luar total</small>{/if}</span>
                  <span class="num amt">{fmtIDR.format(r.spent)}{#if r.limit}<span class="muted"> / {fmtIDR.format(r.limit)}</span>{/if}</span>
                </div>
                {#if r.limit}
                  <div class="bar"><span class="grow" class:warn={p >= 90 && p <= 100} class:over={p > 100} style:width="{Math.min(p, 100)}%"></span></div>
                  <div class="sub">
                    {#if p > 100}<span class="overtxt">Lewat {fmtIDR.format(r.spent - r.limit)}</span>
                    {:else}Sisa {fmtIDR.format(r.limit - r.spent)}{/if}
                  </div>
                {/if}
              </div>
            </li>
          {:else}
            <li class="empty">Belum ada pengeluaran bulan ini.</li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>

  <div class="stack">
    <div class="sec"><h2>Akan datang bulan ini</h2></div>
    <section class="card list">
      {#each upcoming as u (u.recurring_id)}
        {@const due = u.date <= today}
        {@const look = catLook(u.category)}
        <div class="item">
          <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
          <span class="main">
            <span class="title">{u.merchant || u.category}</span>
            <span class="meta">{dshort(u.date)}{due ? ' · jatuh tempo' : ''}{u.more ? ` · +${u.more}× lagi, total ${fmtIDR.format(u.total)}` : ''}{u.auto ? ' · otomatis' : ''}</span>
          </span>
          <span class="num amt" class:pos={u.type === 'in'}>{u.type === 'in' ? '+' : ''}{fmtIDR.format(u.amount)}</span>
          {#if due && !u.auto && s.recurring.find((r) => r.id === u.recurring_id)?.next_date === u.date}
            <span class="btns due">
              <button class="primary sm" onclick={() => act(`/recurring/${u.recurring_id}/advance`, 'POST')}>Catat</button>
              <button class="sm" onclick={() => act(`/recurring/${u.recurring_id}/advance?skip=1`, 'POST')}>Lewati</button>
            </span>
          {/if}
        </div>
      {:else}
        <p class="empty">Tidak ada tagihan lagi bulan ini.</p>
      {/each}
    </section>

    <div class="sec">
      <h2>Transaksi berulang</h2>
      <button onclick={() => (sheet = {})}><Icon name="plus" size={16} />Tambah</button>
    </div>
    <section class="card list">
      {#each s.recurring as r (r.id)}
        {@const look = catLook(r.category)}
        <button class="item rec" onclick={() => (sheet = { rec: r })}>
          <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
          <span class="main">
            <span class="title">{r.merchant || r.category}</span>
            <span class="meta">{r.category} · {freqLabel(r)} · berikutnya {dshort(r.next_date)}{r.auto ? ' · otomatis' : ''}</span>
          </span>
          <span class="num amt" class:pos={r.type === 'in'}>
            {r.currency !== 'IDR' ? fmtCur(r.orig_amount, r.currency) : fmtIDR.format(r.amount)}
          </span>
        </button>
      {:else}
        <p class="empty">Tambahkan gaji, langganan (Claude, internet), cicilan, dll. Tagihan yang akan datang ikut mengurangi sisa budget.</p>
      {/each}
    </section>
  </div>
</div>

{#if sheet}
  <RecurringSheet rec={sheet.rec} onclose={() => (sheet = null)} />
{/if}

<style>
  .cols {
    display: grid;
    gap: 12px;
  }
  @media (min-width: 900px) {
    .cols {
      gap: 24px;
    }
  }
  .sec button {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .btns {
    display: flex;
    gap: 6px;
  }
  .hint {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--muted);
    font-size: 13px;
    margin: 0 0 8px;
  }
  .warn {
    color: var(--warn);
  }
  .edit {
    display: grid;
    grid-template-columns: 1fr 150px;
    align-items: center;
    gap: 12px;
    padding: 6px 0;
    font-size: 14px;
  }
  .name {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
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
  .cat {
    font-weight: 600;
  }
  .cat small {
    color: var(--muted);
    font-weight: 400;
  }
  .amt {
    font-weight: 600;
  }
  .amt .muted {
    font-weight: 400;
  }
  .bar {
    height: 6px;
    border-radius: 3px;
    background: var(--bg);
    margin: 7px 0 3px;
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: 3px;
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
  .item {
    display: flex;
    flex-wrap: wrap;
    width: 100%;
    gap: 8px 12px;
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
  .item:last-child {
    border-bottom: 0;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
  }
  .title {
    font-weight: 600;
  }
  .meta {
    color: var(--muted);
    font-size: 13px;
  }
  .due {
    width: 100%;
    padding-left: 52px;
  }
  .sm {
    padding: 6px 14px;
    font-size: 13px;
  }
  .rec:active {
    scale: none;
    background: color-mix(in srgb, var(--text) 5%, transparent);
  }
</style>
