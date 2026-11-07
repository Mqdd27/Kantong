<script>
  import { onMount } from 'svelte'
  import { s, fmtNum, jakartaLocal, createTx, updateTx, deleteTx } from './lib/store.svelte.js'

  /** tx = undefined → add, otherwise edit */
  let { tx, onclose } = $props()

  let dialog
  let type = $state(tx?.type ?? 'out')
  let category = $state(tx?.category ?? '')
  let merchant = $state(tx?.merchant ?? '')
  let note = $state(tx?.note ?? '')
  let amountText = $state(tx ? fmtNum.format(tx.amount) : '')
  let when = $state(tx ? jakartaLocal(new Date(tx.occurred_at)) : jakartaLocal())

  const cats = $derived(s.categories.filter((c) => c.type === type))
  const amount = $derived(Number(amountText.replace(/\D/g, '')))

  $effect(() => {
    if (cats.length && !cats.some((c) => c.name === category)) category = cats[0].name
  })

  onMount(() => dialog.showModal())

  function onAmount(e) {
    const n = e.currentTarget.value.replace(/\D/g, '')
    amountText = n ? fmtNum.format(Number(n)) : ''
  }

  function save(e) {
    e.preventDefault()
    if (!amount) return
    const fields = { occurred_at: when + ':00+07:00', amount, type, category, merchant, note }
    if (tx) updateTx(tx.id, { ...fields, needs_review: false })
    else createTx({ ...fields, account: '', source: 'manual' })
    dialog.close()
  }

  function del() {
    deleteTx(tx.id)
    dialog.close()
  }
</script>

<dialog bind:this={dialog} onclose={onclose} onclick={(e) => e.target === dialog && dialog.close()}>
  <form onsubmit={save}>
    <h2>{tx ? 'Edit transaksi' : 'Tambah transaksi'}</h2>

    <div class="types">
      {#each [['out', 'Keluar'], ['in', 'Masuk'], ['transfer', 'Transfer']] as [v, label]}
        <button type="button" class:on={type === v} onclick={() => (type = v)}>{label}</button>
      {/each}
    </div>

    <label>Nominal
      <input inputmode="numeric" value={amountText} oninput={onAmount} placeholder="0" required />
    </label>
    <label>Kategori
      <select bind:value={category}>
        {#each cats as c}<option value={c.name}>{c.name}</option>{/each}
      </select>
    </label>
    <label>Merchant
      <input bind:value={merchant} autocomplete="off" />
    </label>
    <label>Catatan
      <input bind:value={note} autocomplete="off" />
    </label>
    <label>Waktu
      <input type="datetime-local" bind:value={when} required />
    </label>

    <div class="actions">
      {#if tx}<button type="button" class="danger" onclick={del}>Hapus</button>{/if}
      <button type="button" onclick={() => dialog.close()}>Batal</button>
      <button class="primary" disabled={!amount}>Simpan</button>
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
  .types {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 6px;
  }
  .types button {
    border: 1px solid var(--line);
    background: var(--surface);
    color: var(--text);
    border-radius: 10px;
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
    margin-top: 4px;
  }
  .danger {
    margin-right: auto;
    color: var(--out);
  }
</style>
