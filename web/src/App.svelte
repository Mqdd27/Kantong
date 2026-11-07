<script>
  import { untrack } from 'svelte'
  import { s, load, connect } from './lib/store.svelte.js'
  import Login from './Login.svelte'
  import Home from './Home.svelte'
  import TxList from './TxList.svelte'
  import Review from './Review.svelte'
  import Assets from './Assets.svelte'
  import TxSheet from './TxSheet.svelte'

  let route = $state(location.hash.slice(1) || '/')
  addEventListener('hashchange', () => (route = location.hash.slice(1) || '/'))

  // null = closed, {} = add, {tx} = edit
  let sheet = $state(null)
  const open = (tx) => (sheet = { tx })

  const reviewCount = $derived(s.txs.filter((t) => t.needs_review).length)

  $effect(() => {
    if (s.authed) untrack(() => { load(); connect() })
  })

  const tabs = [
    ['/', 'Beranda'],
    ['/tx', 'Transaksi'],
    ['/review', 'Review'],
    ['/assets', 'Aset'],
  ]
</script>

{#if !s.authed}
  <Login />
{:else}
  <main>
    {#if route === '/tx'}
      <TxList {open} />
    {:else if route === '/review'}
      <Review {open} />
    {:else if route === '/assets'}
      <Assets />
    {:else}
      <Home {open} />
    {/if}
  </main>

  {#if route !== '/assets'}
    <button class="fab" aria-label="Tambah transaksi" onclick={() => (sheet = {})}>+</button>
  {/if}

  <nav>
    {#each tabs as [href, label]}
      <a href={'#' + href} class:active={route === href}>
        {label}
        {#if href === '/review' && reviewCount}<span class="count">{reviewCount}</span>{/if}
      </a>
    {/each}
  </nav>

  {#if sheet}
    <TxSheet tx={sheet.tx} onclose={() => (sheet = null)} />
  {/if}
{/if}

{#if s.error}<div class="toast" role="alert">{s.error}</div>{/if}

<style>
  main {
    padding: calc(env(safe-area-inset-top) + 12px) 16px calc(env(safe-area-inset-bottom) + 96px);
    max-width: 640px;
    margin: 0 auto;
  }
  nav {
    position: fixed;
    inset: auto 0 0 0;
    display: flex;
    background: var(--surface);
    border-top: 1px solid var(--line);
    padding-bottom: env(safe-area-inset-bottom);
  }
  nav a {
    flex: 1;
    text-align: center;
    padding: 14px 0;
    color: var(--muted);
    text-decoration: none;
    font-size: 14px;
  }
  nav a.active {
    color: var(--text);
    font-weight: 600;
  }
  .count {
    background: var(--warn);
    color: #fff;
    border-radius: 999px;
    padding: 0 6px;
    font-size: 11px;
    margin-left: 4px;
  }
  .fab {
    position: fixed;
    right: 16px;
    bottom: calc(env(safe-area-inset-bottom) + 68px);
    width: 56px;
    height: 56px;
    border-radius: 50%;
    border: 0;
    background: var(--accent);
    color: var(--on-accent);
    font-size: 28px;
    box-shadow: 0 4px 12px rgb(0 0 0 / 0.25);
  }
  .toast {
    position: fixed;
    left: 16px;
    right: 16px;
    top: calc(env(safe-area-inset-top) + 12px);
    background: var(--out);
    color: #fff;
    padding: 12px 14px;
    border-radius: 10px;
    z-index: 10;
  }
</style>
