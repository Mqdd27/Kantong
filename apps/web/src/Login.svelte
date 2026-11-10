<script>
  import { login } from './lib/store.svelte.js'

  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await login(password)
    } catch {
      error = 'Password salah'
    }
    busy = false
  }
</script>

<form onsubmit={submit}>
  <h1>Kantong</h1>
  <label for="pw" class="sr-only">Password</label>
  <input id="pw" type="password" bind:value={password} placeholder="Password" autocomplete="current-password" required />
  <button class="primary" disabled={busy}>Masuk</button>
  {#if error}<p class="err" role="alert">{error}</p>{/if}
</form>

<style>
  form {
    max-width: 320px;
    margin: 30vh auto 0;
    padding: 0 16px;
    display: grid;
    gap: 12px;
  }
  h1 {
    margin: 0 0 8px;
  }
  .err {
    color: var(--out);
    margin: 0;
  }
</style>
