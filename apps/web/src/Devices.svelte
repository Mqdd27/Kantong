<script>
  import { loadSessions, endSession, revokeOthers, logout, deviceName, deviceIcon, fmtShort, loadMe, saveTelegram, changePassword } from './lib/store.svelte.js'
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

  let me = $state(null)
  loadMe().then((m) => (me = m))
  let editing = $state(false)
  let botToken = $state('')
  let chatId = $state('')
  let tgError = $state('')
  async function connect(e, token = botToken.trim(), chat = chatId.trim()) {
    e?.preventDefault()
    busy = true
    tgError = ''
    try {
      me = await saveTelegram(token, chat)
      editing = false
      botToken = chatId = ''
    } catch (err) {
      tgError = err.message
    }
    busy = false
  }

  let pwOpen = $state(false)
  let pwCur = $state('')
  let pwNew = $state('')
  let pwAgain = $state('')
  let pwMsg = $state('')
  const pwText = (m) =>
    m.includes('current password') ? 'Password lama salah'
    : m.includes('too many') ? 'Terlalu banyak percobaan gagal. Coba lagi 15 menit lagi.'
    : m.includes('at least 8') ? 'Password baru minimal 8 karakter'
    : 'Gagal mengganti password, coba lagi'
  async function savePassword(e) {
    e.preventDefault()
    if (pwNew !== pwAgain) return (pwMsg = 'Password baru tidak sama')
    busy = true
    pwMsg = ''
    try {
      await changePassword(pwCur, pwNew)
      pwOpen = false
      pwCur = pwNew = pwAgain = ''
      pwMsg = 'Password diganti. Perangkat lain sudah dikeluarkan.'
      await load()
    } catch (err) {
      pwMsg = pwText(err.message)
    }
    busy = false
  }
</script>

{#snippet device(x)}
  <span class="avatar"><Icon name={deviceIcon(x.user_agent)} /></span>
  <span class="main">
    <b>{deviceName(x.user_agent)}</b>
    <span class="muted">{x.current ? 'online' : 'aktif ' + seen(x)} · {x.ip || 'IP tidak diketahui'}</span>
    <span class="muted">masuk {fmtShort.format(new Date(x.created_at))}</span>
  </span>
{/snippet}

<PageHead title="Akun & perangkat" />

{#if me}
  <h2 class="sec">Akun</h2>
  <section class="card list">
    {#if pwOpen}
      <form class="tg" onsubmit={savePassword}>
        <label>Password lama <input type="password" bind:value={pwCur} autocomplete="current-password" required /></label>
        <label>Password baru <input type="password" bind:value={pwNew} autocomplete="new-password" minlength="8" required /></label>
        <label>Ulangi password baru <input type="password" bind:value={pwAgain} autocomplete="new-password" minlength="8" required /></label>
        <p class="muted">Perangkat lain akan dikeluarkan. Perangkat ini tetap masuk.</p>
        {#if pwMsg}<p class="err" role="alert">{pwMsg}</p>{/if}
        <div class="btns">
          <button type="button" onclick={() => ((pwOpen = false), (pwMsg = ''))}>Batal</button>
          <button class="primary" disabled={busy}>Simpan</button>
        </div>
      </form>
    {:else}
      <div class="row">
        <span class="main"><b>{me.username}</b>{#if pwMsg}<span class="muted" role="status">{pwMsg}</span>{/if}</span>
        <button onclick={() => ((pwOpen = true), (pwMsg = ''))}>Ganti password</button>
      </div>
    {/if}
  </section>

  <h2 class="sec">Notifikasi Telegram</h2>
  <section class="card list">
    {#if editing}
      <form class="tg" onsubmit={connect}>
        {#if me.telegram.server_bot}
          <p class="muted">Cukup isi chat ID: notifikasi dikirim lewat bot Kantong di server. Token bot hanya kalau mau pakai bot sendiri.</p>
        {/if}
        <label>Chat ID <input bind:value={chatId} inputmode="numeric" placeholder="mis. 123456789 (dari @userinfobot)" autocomplete="off" required /></label>
        <label>Token bot{me.telegram.server_bot ? ' (opsional)' : ''} <input bind:value={botToken} placeholder="123456:ABC… dari @BotFather" autocomplete="off" spellcheck="false" required={!me.telegram.server_bot} /></label>
        <p class="muted">Tekan Start di chat dengan bot-nya dulu. Kantong mengirim pesan tes sebelum menyimpan.</p>
        {#if tgError}<p class="err" role="alert">{tgError}</p>{/if}
        <div class="btns">
          <button type="button" onclick={() => ((editing = false), (tgError = ''))}>Batal</button>
          <button class="primary" disabled={busy}>Hubungkan</button>
        </div>
      </form>
    {:else}
      <div class="row">
        <span class="main">
          <b>{me.telegram.connected ? 'Terhubung' : 'Belum terhubung'}</b>
          <span class="muted">
            {#if me.telegram.from_env}Bot dari .env server, chat {me.telegram.chat_id}
            {:else if me.telegram.connected && me.telegram.own_bot}Bot kamu sendiri, chat {me.telegram.chat_id}
            {:else if me.telegram.connected}Bot Kantong di server, chat {me.telegram.chat_id}
            {:else}Transaksi dari email tidak dikirim ke Telegram{/if}
          </span>
        </span>
        {#if me.telegram.connected && !me.telegram.from_env}
          <button class="danger" disabled={busy} onclick={() => connect(null, '', '')}>Putuskan</button>
        {/if}
        <button onclick={() => (editing = true)}>{me.telegram.connected && !me.telegram.from_env ? 'Ganti' : 'Hubungkan bot'}</button>
      </div>
      {#if tgError}<p class="err" role="alert">{tgError}</p>{/if}
    {/if}
  </section>
{/if}

<h2 class="sec">Perangkat ini</h2>
<section class="card list">
  {#if mine}
    <div class="row">
      {@render device(mine)}
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
<p class="muted note">Token API (Hermes, script) tidak ikut keluar.</p>

<h2 class="sec">Sesi aktif lain</h2>
<section class="card list">
  {#each others as x (x.id)}
    <div class="row">
      {@render device(x)}
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
  .row:last-child {
    border-bottom: 0;
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
  .tg {
    display: grid;
    gap: 10px;
    padding: 12px 0;
  }
  .tg label {
    display: grid;
    gap: 4px;
    font-size: 14px;
  }
  .tg p {
    margin: 0;
  }
  .btns {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  .err {
    color: var(--out);
    font-size: 14px;
    margin: 4px 0;
  }
</style>
