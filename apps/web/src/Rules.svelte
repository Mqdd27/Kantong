<script>
  import { api } from './lib/api.js'
  import { act } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'

  let rules = $state([])
  const load = () => api('/rules').then((r) => (rules = r)).catch(() => {})
  load()
  async function del(id) {
    if (await act(`/rules/${id}`, 'DELETE')) load()
  }
</script>

<PageHead title="Aturan kategori" />
<p class="muted intro">Email dengan merchant yang mengandung teks ini langsung masuk ke kategorinya, tanpa review. Aturan dibuat dari koreksi di Review atau saat edit transaksi.</p>

<section class="card list">
{#each rules as r (r.id)}
  {@const look = catLook(r.category)}
  <div class="row">
    <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
    <span class="main"><b>{r.pattern}</b><span class="muted">{r.type === 'in' ? 'Masuk' : r.type === 'out' ? 'Keluar' : 'Transfer'} · {r.category}</span></span>
    <button onclick={() => del(r.id)}>Hapus</button>
  </div>
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
</style>
