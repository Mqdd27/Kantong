<script>
  import { onMount } from 'svelte'
  import { s, fmtNum, fmtIDR, CURRENCIES, jakartaLocal, createTx, updateTx, deleteTx, addRule, loadAssets, act } from './lib/store.svelte.js'

  /** tx = undefined → add, otherwise edit */
  let { tx, onclose } = $props()

  let dialog
  let type = $state(tx?.type ?? 'out')
  let category = $state(tx?.category ?? '')
  let merchant = $state(tx?.merchant ?? '')
  let note = $state(tx?.note ?? '')
  let account = $state(tx?.account ?? '')
  let toAccount = $state(tx?.to_account ?? '')
  let amountText = $state(tx ? fmtNum.format(tx.amount) : '')
  let currency = $state(tx?.currency ?? 'IDR')
  let orig = $state(tx?.orig_amount ?? '')
  const foreign = $derived(currency !== 'IDR')
  let when = $state(tx ? jakartaLocal(new Date(tx.occurred_at)) : jakartaLocal())

  const cats = $derived(s.categories.filter((c) => c.type === type))
  const corrected = $derived(tx && merchant.trim() && (category !== tx.category || type !== tx.type))
  let always = $state(false)
  const amount = $derived(Number(amountText.replace(/\D/g, '')))

  $effect(() => {
    if (cats.length && !cats.some((c) => c.name === category)) category = cats[0].name
  })

  onMount(() => {
    dialog.showModal()
    if (!s.portfolio.assets.length) loadAssets() // account suggestions
  })

  function onAmount(e) {
    const n = e.currentTarget.value.replace(/\D/g, '')
    amountText = n ? fmtNum.format(Number(n)) : ''
  }

  function save(e) {
    e.preventDefault()
    if (foreign ? !(orig > 0) : !amount) return
    const fields = { occurred_at: when + ':00+07:00', type, category, merchant, note, currency, account, to_account: type === 'transfer' ? toAccount : '' }
    if (foreign) fields.orig_amount = Number(orig)
    if (amount) fields.amount = amount // foreign + empty IDR: server converts at that day's rate
    if (corrected && always) addRule(merchant, type, category)
    if (tx) updateTx(tx.id, { ...fields, needs_review: false })
    else createTx({ ...fields, source: 'manual' })
    dialog.close()
  }

  function del() {
    deleteTx(tx.id)
    dialog.close()
  }

  // ---- split: one payment over several categories (IDR in/out only) ----
  const canSplit = $derived(tx && tx.type !== 'transfer' && (tx.currency ?? 'IDR') === 'IDR')
  const group = $derived(tx?.split_of ? s.txs.filter((x) => x.split_of === tx.split_of).sort((a, b) => a.id - b.id) : tx ? [tx] : [])
  const total = $derived(group.reduce((n, x) => n + x.amount, 0))
  let splitting = $state(false)
  let parts = $state([])
  function startSplit() {
    parts = group.map((x) => ({ category: x.category, amount: fmtNum.format(x.amount), note: x.note }))
    if (parts.length === 1) parts.push({ category: cats.find((c) => c.name !== tx.category)?.name ?? tx.category, amount: '', note: '' })
    splitting = true
  }
  const num = (t) => Number(String(t).replace(/\D/g, ''))
  const left = $derived(total - parts.reduce((n, p) => n + num(p.amount), 0))
  const typeAmount = (p, e) => { const n = e.currentTarget.value.replace(/\D/g, ''); p.amount = n ? fmtNum.format(Number(n)) : '' }
  async function sendSplit(list) {
    if (await act(`/transactions/${tx.id}/split`, 'POST', { parts: list })) dialog.close()
  }
  function saveSplit(e) {
    e.preventDefault()
    if (left === 0) sendSplit(parts.filter((p) => num(p.amount)).map((p) => ({ category: p.category, amount: num(p.amount), note: p.note })))
  }
  const merge = () => sendSplit([{ category: group[0].category, amount: total, note: group[0].note }])
</script>

<dialog class="sheet" bind:this={dialog} onclose={onclose} onclick={(e) => e.target === dialog && dialog.close()}>
  {#if splitting}
  <form onsubmit={saveSplit}>
    <h2>Split · {tx.merchant || tx.category} {fmtIDR.format(total)}</h2>
    {#each parts as p, i}
      <div class="part">
        <select bind:value={p.category} aria-label="Kategori bagian {i + 1}">
          {#each cats as c}<option value={c.name}>{c.name}</option>{/each}
        </select>
        <input inputmode="numeric" value={p.amount} oninput={(e) => typeAmount(p, e)} placeholder="0" aria-label="Nominal bagian {i + 1}" />
        <button type="button" class="x" aria-label="Hapus bagian {i + 1}" disabled={parts.length < 2} onclick={() => parts.splice(i, 1)}>×</button>
        <input class="pnote" bind:value={p.note} placeholder="catatan (opsional)" autocomplete="off" aria-label="Catatan bagian {i + 1}" />
      </div>
    {/each}
    <div class="sum">
      <button type="button" onclick={() => parts.push({ category: cats[0]?.name ?? '', amount: left > 0 ? fmtNum.format(left) : '', note: '' })}>+ Bagian</button>
      <span class:bad={left !== 0}>{left === 0 ? 'Pas' : left > 0 ? `Sisa ${fmtIDR.format(left)}` : `Lebih ${fmtIDR.format(-left)}`}</span>
    </div>
    <div class="actions">
      {#if tx.split_of}<button type="button" onclick={merge}>Gabung lagi</button>{/if}
      <button type="button" onclick={() => (splitting = false)}>Kembali</button>
      <button class="primary" disabled={left !== 0}>Simpan split</button>
    </div>
  </form>
  {:else}
  <form onsubmit={save}>
    <h2>{tx ? 'Edit transaksi' : 'Tambah transaksi'}</h2>

    <div class="types seg">
      {#each [['out', 'Keluar'], ['in', 'Masuk'], ['transfer', 'Transfer']] as [v, label]}
        <button type="button" aria-pressed={type === v} onclick={() => (type = v)}>{label}</button>
      {/each}
    </div>

    <div class="amt">
      <label>Mata uang
        <select bind:value={currency} onchange={() => (amountText = '')}>
          {#each CURRENCIES as c}<option>{c}</option>{/each}
        </select>
      </label>
      {#if foreign}
        <label>Nominal {currency}
          <input type="number" inputmode="decimal" step="any" min="0" bind:value={orig} oninput={() => (amountText = '')} required />
        </label>
      {:else}
        <label>Nominal
          <input inputmode="numeric" value={amountText} oninput={onAmount} placeholder="0" required />
        </label>
      {/if}
    </div>
    {#if foreign}
      <label>Terpotong (Rp), kosongkan = pakai kurs hari itu
        <input inputmode="numeric" value={amountText} oninput={onAmount} placeholder="otomatis" />
      </label>
    {/if}
    <label>Kategori
      <select bind:value={category}>
        {#each cats as c}<option value={c.name}>{c.name}</option>{/each}
      </select>
    </label>
    <label>Merchant
      <input bind:value={merchant} autocomplete="off" />
    </label>
    {#if corrected}
      <label class="check"><input type="checkbox" bind:checked={always} /> Selalu kategorikan “{merchant.trim()}” sebagai {category}</label>
    {/if}
    <div class:two={type === 'transfer'}>
      <label>{type === 'transfer' ? 'Dari akun' : 'Akun'}
        <input bind:value={account} list="accounts" autocomplete="off" placeholder="mis. Jago" />
      </label>
      {#if type === 'transfer'}
        <label>Ke akun
          <input bind:value={toAccount} list="accounts" autocomplete="off" placeholder="kosong = ke orang lain" />
        </label>
      {/if}
    </div>
    <datalist id="accounts">
      {#each s.portfolio.assets as a}<option value={a.name}></option>{/each}
    </datalist>
    <label>Catatan
      <input bind:value={note} autocomplete="off" />
    </label>
    <label>Waktu
      <input type="datetime-local" bind:value={when} required />
    </label>

    <div class="actions">
      {#if tx}<button type="button" class="danger" onclick={del}>Hapus</button>{/if}
      {#if canSplit}<button type="button" onclick={startSplit}>{tx.split_of ? `Split (${group.length})` : 'Split'}</button>{/if}
      <button type="button" onclick={() => dialog.close()}>Batal</button>
      <button class="primary" disabled={foreign ? !(orig > 0) : !amount}>Simpan</button>
    </div>
  </form>
  {/if}
</dialog>

<style>
  .check {
    display: flex;
    gap: 8px;
    align-items: center;
    color: var(--text);
    font-size: 14px;
  }
  .check input {
    width: 20px;
    height: 20px;
    margin: 0;
  }
  .amt {
    display: grid;
    grid-template-columns: 96px 1fr;
    gap: 8px;
  }
  .part {
    display: grid;
    grid-template-columns: 1fr 1fr 40px;
    gap: 6px;
    padding-bottom: 10px;
    border-bottom: 1px solid var(--line);
  }
  .part .pnote {
    grid-column: 1 / -1;
  }
  .x {
    padding: 0;
    font-size: 20px;
  }
  .sum {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 14px;
    font-weight: 600;
    color: var(--in);
  }
  .sum .bad {
    color: var(--warn);
  }
</style>
