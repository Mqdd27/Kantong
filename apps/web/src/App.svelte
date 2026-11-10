<script>
  import { untrack } from 'svelte'
  import { s, load, connect } from './lib/store.svelte.js'
  import Login from './Login.svelte'
  import Home from './Home.svelte'
  import TxList from './TxList.svelte'
  import Review from './Review.svelte'
  import Assets from './Assets.svelte'
  import Budget from './Budget.svelte'
  import Analytics from './Analytics.svelte'
  import Rules from './Rules.svelte'
  import Calendar from './Calendar.svelte'
  import TxSheet from './TxSheet.svelte'

  let route = $state(location.hash.slice(1) || '/')
  addEventListener('hashchange', () => (route = location.hash.slice(1) || '/'))

  // null = closed, {} = add, {tx} = edit
  let sheet = $state(null)
  const open = (tx) => (sheet = { tx })

  let wide = $state(matchMedia('(min-width: 900px)').matches)
  matchMedia('(min-width: 900px)').addEventListener('change', (e) => (wide = e.matches))
  const reviewCount = $derived(s.txs.filter((t) => t.needs_review).length)

  $effect(() => {
    if (s.authed) untrack(() => { load(); connect() })
  })

  const tabs = [
    ['/', 'Beranda'],
    ['/tx', 'Transaksi'],
    ['/calendar', 'Kalender'], // mobile: toggle on Transaksi
    ['/budget', 'Budget'],
    ['/analytics', 'Analitik'],
    ['/assets', 'Aset'],
    ['/review', 'Review'], // mobile reaches it through the banner on Beranda/Transaksi
    ['/rules', 'Aturan'], // mobile: link on Transaksi
  ]
</script>

{#if !s.authed}
  <Login />
{:else}
  <main class:narrow={route === '/tx' || route === '/review' || route === '/rules' || route === '/calendar'}>
    {#if reviewCount && (route === '/' || route === '/tx')}
      <a class="banner" href="#/review">⚠︎ {reviewCount} transaksi perlu direview ›</a>
    {/if}
    {#if route === '/tx'}
      <TxList {open} />
    {:else if route === '/review'}
      <Review {open} />
    {:else if route === '/assets'}
      <Assets />
    {:else if route === '/budget'}
      <Budget />
    {:else if route === '/analytics'}
      <Analytics />
    {:else if route === '/calendar'}
      <Calendar {open} />
    {:else if route === '/rules'}
      <Rules />
    {:else}
      <Home {open} />
    {/if}
  </main>

  {#if route !== '/assets' && route !== '/analytics' && route !== '/rules'}
    <button class="fab" aria-label="Tambah transaksi" onclick={() => (sheet = {})}>+</button>
  {/if}

  <nav>
    <span class="brand">Kantong</span>
    <button class="primary new" onclick={() => (sheet = {})}>+ Transaksi</button>
    {#each tabs as [href, label]}
      {@const active = route === href || (href === '/tx' && route === '/calendar' && !wide)}
      <a href={'#' + href} aria-current={active ? 'page' : undefined} class:active class:desk={href === '/review' || href === '/rules' || href === '/calendar'}>
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
  .brand,
  .new,
  .desk {
    display: none;
  }
  @media (min-width: 900px) {
    main {
      max-width: 1200px;
      margin-left: 220px;
      padding: 32px 40px;
    }
    main.narrow {
      max-width: 800px;
    }
    nav {
      inset: 0 auto 0 0;
      width: 220px;
      flex-direction: column;
      gap: 2px;
      padding: 24px 12px;
      border-top: 0;
      border-right: 1px solid var(--line);
    }
    nav a,
    nav a.desk {
      display: block;
      flex: none;
      text-align: left;
      padding: 10px 12px;
      border-radius: 8px;
      font-size: 15px;
    }
    nav a.active {
      background: var(--bg);
    }
    .brand {
      display: block;
      font-weight: 700;
      font-size: 18px;
      padding: 0 12px 16px;
    }
    .new {
      display: block;
      margin-bottom: 16px;
    }
    .fab {
      display: none;
    }
    .toast {
      left: auto;
      width: 360px;
    }
  }
  .banner {
    display: block;
    background: var(--surface);
    border: 1px solid var(--warn);
    color: var(--text);
    border-radius: 10px;
    padding: 10px 14px;
    margin-bottom: 12px;
    text-decoration: none;
    font-size: 14px;
  }
  .count {
    background: var(--warn);
    color: var(--on-warn);
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
    color: var(--on-out);
    padding: 12px 14px;
    border-radius: 10px;
    z-index: 10;
  }
</style>
