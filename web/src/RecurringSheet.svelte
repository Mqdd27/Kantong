<script>
  import { onMount } from 'svelte'
  import { s, fmtNum, CURRENCIES, act } from './lib/store.svelte.js'

  /** rec = undefined → add, otherwise edit */
  let { rec, onclose } = $props()

  let dialog
  let type = $state(rec?.type ?? 'out')
  let category = $state(rec?.category ?? 'Tagihan')
  let merchant = $state(rec?.merchant ?? '')
  let currency = $state(rec?.currency ?? 'IDR')
  let orig = $state(rec?.orig_amount ?? '')
  let amountText = $state(rec?.amount ? fmtNum.format(rec.amount) : '')
  let freq = $state(rec?.freq ?? 'monthly')
  let next = $state(rec?.next_date ?? new Date(Date.now() + 7 * 3600e3).toISOString().slice(0, 10))
  let auto = $state(rec?.auto ?? false)
  let note = $state(rec?.note ?? '')

  const foreign = $derived(currency !== 'IDR')
  const amount = $derived(Number(amountText.replace(/\D/g, '')))
  const cats = $derived(s.categories.filter((c) => c.type === type))
  $effect(() => {
    if (cats.length && !cats.some((c) => c.name === category)) category = cats[0].name
  })
  onMount(() => dialog.showModal())

  async function save(e) {
    e.preventDefault()
    const body = { type, category, merchant, currency, freq, next_date: next, auto, note, amount: foreign ? 0 : amount }
    if (foreign) body.orig_amount = Number(orig)
    if (await act(rec ? `/recurring/${rec.id}` : '/recurring', rec ? 'PATCH' : 'POST', body)) dialog.close()
  }
  async function del() {
    if (await act(`/recurring/${rec.id}`, 'DELETE')) dialog.close()
  }
</script>

<dialog bind:this={dialog} onclose={onclose} onclick={(e) => e.target === dialog && dialog.close()}>
  <form onsubmit={save}>
    <h2>{rec ? 'Edit transaksi berulang' : 'Transaksi berulang baru'}</h2>
    <div class="types">
      {#each [['out', 'Keluar'], ['in', 'Masuk']] as [v, label]}
        <button type="button" class:on={type === v} onclick={() => (type = v)}>{label}</button>
      {/each}
    </div>
    <label>Nama <input bind:value={merchant} placeholder="mis. Claude Pro, Gaji, Internet" required autocomplete="off" /></label>
    <div class="two">
      <label>Mata uang
        <select bind:value={currency}>{#each CURRENCIES as c}<option>{c}</option>{/each}</select>
      </label>
      {#if foreign}
        <label>Nominal {currency}
          <input type="number" inputmode="decimal" step="any" min="0" bind:value={orig} required />
        </label>
      {:else}
        <label>Nominal
          <input inputmode="numeric" value={amountText} required placeholder="0"
            oninput={(e) => { const n = e.currentTarget.value.replace(/\D/g, ''); amountText = n ? fmtNum.format(Number(n)) : '' }} />
        </label>
      {/if}
    </div>
    <div class="two">
      <label>Kategori
        <select bind:value={category}>{#each cats as c}<option value={c.name}>{c.name}</option>{/each}</select>
      </label>
      <label>Ulangi
        <select bind:value={freq}>
          <option value="weekly">Tiap minggu</option>
          <option value="monthly">Tiap bulan</option>
          <option value="yearly">Tiap tahun</option>
        </select>
      </label>
    </div>
    <label>Tanggal berikutnya <input type="date" bind:value={next} required /></label>
    <label class="check">
      <input type="checkbox" bind:checked={auto} />
      <span>Catat otomatis di tanggalnya<br /><small>Matikan kalau transaksinya sudah tercatat dari email bank, biar nggak dobel.</small></span>
    </label>
    <label>Catatan <input bind:value={note} autocomplete="off" /></label>
    <div class="actions">
      {#if rec}<button type="button" class="danger" onclick={del}>Hapus</button>{/if}
      <button type="button" onclick={() => dialog.close()}>Batal</button>
      <button class="primary">Simpan</button>
    </div>
  </form>
</dialog>

<style>
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
  .check {
    grid-template-columns: auto 1fr;
    align-items: start;
    gap: 10px;
    color: var(--text);
    font-size: 14px;
  }
  .check input {
    width: 20px;
    height: 20px;
    margin: 0;
  }
  .check small {
    color: var(--muted);
  }
  .types {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
  }
  .types button {
    padding: 10px 0;
  }
  .types .on {
    border-color: var(--accent);
    font-weight: 600;
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
