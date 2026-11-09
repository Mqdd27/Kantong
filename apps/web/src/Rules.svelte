<script>
  import { api } from './lib/api.js'
  import { act } from './lib/store.svelte.js'

  let rules = $state([])
  const load = () => api('/rules').then((r) => (rules = r)).catch(() => {})
  load()
  async function del(id) {
    if (await act(`/rules/${id}`, 'DELETE')) load()
  }
</script>

<h2>Aturan kategori</h2>
<p class="muted">Email dengan merchant yang mengandung teks ini langsung masuk ke kategorinya, tanpa review. Aturan dibuat dari koreksi di Review atau saat edit transaksi.</p>

{#each rules as r (r.id)}
  <div class="row">
    <span class="main"><b>{r.pattern}</b><span class="muted">{r.type === 'in' ? 'Masuk' : r.type === 'out' ? 'Keluar' : 'Transfer'} · {r.category}</span></span>
    <button onclick={() => del(r.id)}>Hapus</button>
  </div>
{:else}
  <p class="muted">Belum ada aturan.</p>
{/each}

<style>
  h2 {
    margin: 0 0 4px;
    font-size: 20px;
  }
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 0;
    border-bottom: 1px solid var(--line);
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
</style>
