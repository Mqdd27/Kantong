<script>
  import { loadSessions, endSession, revokeOthers, logout, deviceName, deviceIcon, fmtShort } from './lib/store.svelte.js'
  import PageHead from './PageHead.svelte'
  import Icon from './Icon.svelte'

  let list = $state(null)
  const load = async () => (list = await loadSessions())
  load()
  const mine = $derived(list?.find((x) => x.current))
  const others = $derived(list?.filter((x) => !x.current) ?? [])

  let confirming = $state(false)
  let busy = $state(false)
  async function run(fn) {
    busy = true
    if (await fn()) await load()
    busy = false
  }
  const seen = (x) => fmtShort.format(new Date(x.last_seen))
</script>

{#snippet device(x)}
  <span class="avatar"><Icon name={deviceIcon(x.user_agent)} /></span>
  <span class="main">
    <b>{deviceName(x.user_agent)}</b>
    <span class="muted">{x.ip || 'IP tidak diketahui'} · masuk {fmtShort.format(new Date(x.created_at))}</span>
  </span>
{/snippet}

<PageHead title="Perangkat" />

<h2 class="sec">Perangkat ini</h2>
<section class="card list">
  {#if mine}
    <div class="row">
      {@render device(mine)}
      <span class="muted when">online</span>
    </div>
  {/if}
  <button class="danger wide" onclick={logout}><Icon name="logout" size={18} />Keluar</button>
  {#if others.length}
    {#if confirming}
      <div class="row">
        <span class="main"><b>Keluarkan {others.length} perangkat lain?</b><span class="muted">Perangkat itu harus login ulang.</span></span>
        <button class="danger" disabled={busy} onclick={() => run(revokeOthers).then(() => (confirming = false))}>Keluarkan</button>
        <button onclick={() => (confirming = false)}>Batal</button>
      </div>
    {:else}
      <button class="danger wide" onclick={() => (confirming = true)}>Keluar dari semua perangkat lain</button>
    {/if}
  {/if}
</section>
<p class="muted note">Token API (Hermes, script) tidak ikut keluar. Ganti API_TOKEN di .env untuk memutusnya.</p>

<h2 class="sec">Sesi aktif lain</h2>
<section class="card list">
  {#each others as x (x.id)}
    <div class="row">
      {@render device(x)}
      <span class="muted when">{seen(x)}</span>
      <button class="danger" disabled={busy} onclick={() => run(() => endSession(x))}>Keluarkan</button>
    </div>
  {:else}
    <p class="empty">{list ? 'Tidak ada perangkat lain yang login.' : 'Memuat…'}</p>
  {/each}
</section>

<style>
  .muted {
    color: var(--muted);
    font-size: 13px;
  }
  .sec {
    margin: 16px 0 8px;
    font-size: 15px;
    font-weight: 650;
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
  .when {
    white-space: nowrap;
  }
  button {
    padding: 6px 12px;
    font-size: 14px;
  }
  .danger {
    color: var(--out);
  }
  .wide {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    margin-top: 8px;
    justify-content: center;
  }
  .note {
    margin: 6px 4px 0;
  }
</style>
