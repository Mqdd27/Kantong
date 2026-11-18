<script>
  import { api } from './lib/api.js'
  import { s, act, editRule, renameCategory, deleteCategory, addCategory } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'

  let rules = $state([])
  const load = () => api('/rules').then((r) => (rules = r)).catch(() => {})
  load()
  async function del(id) {
    if (await act(`/rules/${id}`, 'DELETE')) load()
  }
  let editing = $state(null)

  // categories you created can be renamed; built-in ones are fixed because code refers to their names
  const mine = $derived(s.categories.filter((c) => c.custom))
  let renaming = $state(null) // category id being renamed
  let nameDraft = $state('')
  let renameErr = $state('')
  let renameBusy = $state(false)
  function startRename(c) {
    renaming = c.id
    nameDraft = c.name
    renameErr = ''
  }
  async function saveRename(c) {
    const name = nameDraft.trim()
    if (!name || renameBusy) return
    if (name === c.name) return void (renaming = null)
    renameBusy = true
    const r = await renameCategory(c.id, name)
    renameBusy = false
    if (r.category) {
      renaming = null
      addErr = ''
      load() // rules show category names too
    } else renameErr = r.error
  }
  let removing = $state(null) // category id waiting for the delete confirmation
  async function removeCategory(c) {
    renameBusy = true
    const r = await deleteCategory(c.id)
    renameBusy = false
    removing = null
    if (r.error) renameErr = r.error
    else (load(), (addErr = '')) // its rules are gone; a stale "already exists" no longer holds
  }
  let newName = $state('')
  let newType = $state('out')
  let addErr = $state('')
  async function add() {
    const name = newName.trim()
    if (!name || renameBusy) return
    renameBusy = true
    const r = await addCategory(name, newType)
    renameBusy = false
    if (r.category) newName = ''
    else addErr = r.error
  }
  async function setCategory(r, category) {
    if (category !== r.category && (await editRule(r.id, { category }))) await load()
    editing = null
  }
</script>

<PageHead title="Aturan kategori" />
<p class="muted intro">Email dengan merchant yang mengandung teks ini langsung masuk ke kategorinya, tanpa review. Aturan dibuat dari koreksi di Review atau saat edit transaksi.</p>

<h2 class="sec">Kategori buatanmu</h2>
<section class="card list">
  {#each mine as c (c.id)}
    {@const look = catLook(c.name)}
    <div class="row">
      <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
      {#if renaming === c.id}
        <form class="rename" onsubmit={(e) => (e.preventDefault(), saveRename(c))}>
          <input
            bind:value={nameDraft}
            maxlength="30"
            aria-label="Nama kategori"
            aria-invalid={renameErr ? 'true' : undefined}
            aria-describedby={renameErr ? 'rename-err' : undefined}
            oninput={() => (renameErr = '')}
            autocomplete="off"
          />
          <button class="primary" disabled={!nameDraft.trim() || renameBusy}>Simpan</button>
          <button type="button" onclick={() => (renaming = null)}>Batal</button>
        </form>
      {:else if removing === c.id}
        <span class="main"><b>Hapus {c.name}?</b><span class="muted">Transaksinya pindah ke Lainnya, aturannya ikut terhapus.</span></span>
        <button disabled={renameBusy} onclick={() => removeCategory(c)}>Hapus</button>
        <button class="edit" onclick={() => (removing = null)}>Batal</button>
      {:else}
        <span class="main"><b>{c.name}</b><span class="muted">{c.type === 'in' ? 'Masuk' : 'Keluar'}</span></span>
        <button class="edit" onclick={() => startRename(c)}>Ubah</button>
        <button onclick={() => ((removing = c.id), (renaming = null), (renameErr = ''))}>Hapus</button>
      {/if}
    </div>
    {#if (renaming === c.id || removing === c.id) && renameErr}<p class="rename-err" id="rename-err" role="alert">{renameErr}</p>{/if}
  {:else}
    <p class="empty">Belum ada. Kategori bawaan tidak bisa diubah; tambahkan sendiri di bawah.</p>
  {/each}
  <form class="rename add" onsubmit={(e) => (e.preventDefault(), add())}>
    <input
      bind:value={newName}
      maxlength="30"
      placeholder="Kategori baru"
      aria-label="Nama kategori baru"
      aria-invalid={addErr ? 'true' : undefined}
      aria-describedby={addErr ? 'add-err' : undefined}
      oninput={() => (addErr = '')}
      autocomplete="off"
    />
    <select bind:value={newType} aria-label="Jenis kategori" onchange={() => (addErr = '')}>
      <option value="out">Keluar</option>
      <option value="in">Masuk</option>
    </select>
    <button class="primary" disabled={!newName.trim() || renameBusy}>Tambah</button>
  </form>
  {#if addErr}<p class="rename-err" id="add-err" role="alert">{addErr}</p>{/if}
</section>
<h2 class="sec">Aturan</h2>

<section class="card list">
{#each rules as r (r.id)}
  {@const look = catLook(r.category)}
  <div class="row">
    <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
    <span class="main"><b>{r.pattern}</b><span class="muted">{r.type === 'in' ? 'Masuk' : r.type === 'out' ? 'Keluar' : 'Transfer'} · {r.category}</span></span>
    <button class="edit" onclick={() => (editing = editing === r.id ? null : r.id)}>{editing === r.id ? 'Batal' : 'Ubah'}</button>
    <button onclick={() => del(r.id)}>Hapus</button>
  </div>
  {#if editing === r.id}
    <div class="chips">
      {#each s.categories.filter((c) => c.type === r.type) as c}
        <button class:current={c.name === r.category} onclick={() => setCategory(r, c.name)}>{c.name}</button>
      {/each}
    </div>
  {/if}
{:else}
  <p class="empty">Belum ada aturan.</p>
{/each}
</section>

<style>
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .intro {
    margin: -4px 0 12px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 0;
    border-bottom: 1px solid var(--line);
  }
  .row:last-child {
    border-bottom: 0;
  }
  .main {
    flex: 1;
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  button {
    padding: 6px 12px;
    font-size: 14px;
    color: var(--out);
  }
  .sec {
    margin: 16px 0 8px;
    font-size: 15px;
    font-weight: 650;
  }
  .rename {
    flex: 1;
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 8px;
    align-items: center;
    min-width: 0;
  }
  .rename input[aria-invalid='true'],
  .rename input[aria-invalid='true']:focus {
    border-color: var(--out);
    outline-color: var(--out);
  }
  .add {
    padding: 10px 0;
  }
  .rename-err {
    margin: 0 0 8px;
    color: var(--out);
    font-size: 13px;
  }
  .rename button {
    color: inherit;
  }
  button.edit {
    color: inherit;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding: 0 0 12px;
    border-bottom: 1px solid var(--line);
  }
  .chips button {
    color: inherit;
  }
  .chips button.current {
    font-weight: 600;
    border-color: currentColor;
  }
</style>
