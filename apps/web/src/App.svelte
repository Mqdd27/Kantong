<script>
  import { untrack } from 'svelte'
  import { s, load, connect } from './lib/store.svelte.js'
  import Icon from './Icon.svelte'
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
  import Chat from './Chat.svelte'
  import { api } from './lib/api.js'

  let route = $state(location.hash.slice(1) || '/')
  addEventListener('hashchange', () => (route = location.hash.slice(1) || '/'))

  // null = closed, {} = add, {tx} = edit
  let sheet = $state(null)
  const open = (tx) => (sheet = { tx })

  const reviewCount = $derived(s.txs.filter((t) => t.needs_review).length)

  $effect(() => {
    if (s.authed) untrack(() => {
      load()
      connect()
      api('/chat').then((r) => (s.chat = r.enabled)).catch(() => {})
    })
  })

  // which tab lights up for routes that live under another one
  const PARENT = { '/calendar': '/tx', '/review': '/tx', '/rules': '/tx', '/analytics': '/', '/chat': '/' }
  const here = $derived(PARENT[route] ?? route)

  // phone: four tabs around the add button; the rest is reached from these pages
  const left = [['/', 'Beranda', 'home'], ['/tx', 'Transaksi', 'list']]
  const right = [['/budget', 'Budget', 'pie'], ['/assets', 'Aset', 'wallet']]
  // desktop sidebar: everything
  const side = [
    ['/', 'Beranda', 'home'],
    ['/tx', 'Transaksi', 'list'],
    ['/calendar', 'Kalender', 'calendar'],
    ['/budget', 'Budget', 'pie'],
    ['/analytics', 'Analitik', 'chart'],
    ['/assets', 'Aset', 'wallet'],
  ]
  const more = $derived([...(s.chat ? [['/chat', 'Asisten', 'chat']] : []), ['/review', 'Review', 'inbox'], ['/rules', 'Aturan kategori', 'tag']])
</script>

{#if !s.authed}
  <Login />
{:else}
  <aside>
    <a class="brand" href="#/"><img src="/favicon.png" alt="" width="28" height="28" />Kantong</a>
    <button class="primary new" onclick={() => (sheet = {})}><Icon name="plus" size={18} />Transaksi</button>
    <nav aria-label="Menu utama">
      {#each side as [href, label, icon]}
        <a href={'#' + href} aria-current={route === href ? 'page' : undefined}><Icon name={icon} />{label}</a>
      {/each}
    </nav>
    <p class="group">Lainnya</p>
    <nav aria-label="Menu lainnya">
      {#each more as [href, label, icon]}
        <a href={'#' + href} aria-current={route === href ? 'page' : undefined}>
          <Icon name={icon} />{label}
          {#if href === '/review' && reviewCount}<span class="count">{reviewCount}</span>{/if}
        </a>
      {/each}
      <a href="/api/export.xlsx" download><Icon name="download" />Export Excel</a>
    </nav>
  </aside>

  <main class:narrow={route === '/tx' || route === '/review' || route === '/rules' || route === '/calendar' || route === '/chat'}>
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
    {:else if route === '/chat' && s.chat}
      <Chat />
    {:else}
      <Home {open} />
    {/if}
  </main>

  <nav class="tabs" aria-label="Menu utama">
    {#each left as [href, label, icon]}
      <a href={'#' + href} aria-current={here === href ? 'page' : undefined}><Icon name={icon} size={22} /><span>{label}</span></a>
    {/each}
    <button class="add" aria-label="Tambah transaksi" onclick={() => (sheet = {})}><Icon name="plus" size={26} /></button>
    {#each right as [href, label, icon]}
      <a href={'#' + href} aria-current={here === href ? 'page' : undefined}><Icon name={icon} size={22} /><span>{label}</span></a>
    {/each}
  </nav>

  {#if sheet}
    <TxSheet tx={sheet.tx} onclose={() => (sheet = null)} />
  {/if}
{/if}

{#if s.error}<div class="toast" role="alert">{s.error}</div>{/if}

<style>
  main {
    padding: calc(env(safe-area-inset-top) + 12px) 16px calc(env(safe-area-inset-bottom) + 100px);
    max-width: 640px;
    margin: 0 auto;
  }

  /* ---- phone tab bar ---- */
  .tabs {
    position: fixed;
    inset: auto 0 0 0;
    z-index: 5;
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    align-items: end;
    padding: 6px 4px calc(env(safe-area-inset-bottom) + 6px);
    background: var(--surface);
    border-top: 1px solid var(--line);
  }
  .tabs a {
    display: grid;
    justify-items: center;
    gap: 2px;
    padding: 4px 0;
    color: var(--muted);
    text-decoration: none;
    font-size: 11px;
    font-weight: 500;
  }
  .tabs a[aria-current='page'] {
    color: var(--accent);
    font-weight: 650;
  }
  .add {
    justify-self: center;
    display: grid;
    place-items: center;
    width: 54px;
    height: 54px;
    margin-top: -22px;
    padding: 0;
    border: 4px solid var(--bg);
    border-radius: 50%;
    background: var(--accent);
    color: var(--on-accent);
    box-shadow: 0 4px 14px color-mix(in srgb, var(--accent) 40%, transparent);
  }

  /* ---- desktop sidebar ---- */
  aside {
    display: none;
  }
  @media (min-width: 900px) {
    .tabs {
      display: none;
    }
    aside {
      position: fixed;
      inset: 0 auto 0 0;
      width: 240px;
      display: flex;
      flex-direction: column;
      gap: 2px;
      padding: 20px 14px;
      background: var(--surface);
      border-right: 1px solid var(--line);
      overflow-y: auto;
    }
    main {
      max-width: 1180px;
      margin-left: 240px;
      padding: 28px 40px 48px;
    }
    main.narrow {
      max-width: 820px;
    }
    .toast {
      left: auto;
      width: 360px;
    }
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 10px 18px;
    color: var(--text);
    font-size: 19px;
    font-weight: 700;
    text-decoration: none;
  }
  .brand img {
    border-radius: 8px;
  }
  .new {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    margin-bottom: 16px;
  }
  aside nav {
    display: grid;
    gap: 2px;
  }
  aside nav a {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 9px 10px;
    border-radius: 10px;
    color: var(--muted);
    text-decoration: none;
    font-size: 15px;
    font-weight: 500;
  }
  aside nav a:hover {
    color: var(--text);
    background: var(--bg);
  }
  aside nav a[aria-current='page'] {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, var(--surface));
    font-weight: 650;
  }
  .group {
    margin: 18px 10px 4px;
    color: var(--muted);
    font-size: 12px;
    font-weight: 600;
  }
  .count {
    margin-left: auto;
    min-width: 20px;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--warn);
    color: var(--on-warn);
    font-size: 12px;
    text-align: center;
  }

  .toast {
    position: fixed;
    left: 16px;
    right: 16px;
    top: calc(env(safe-area-inset-top) + 12px);
    background: var(--out);
    color: var(--on-out);
    padding: 12px 14px;
    border-radius: 12px;
    z-index: 10;
    animation: toast-in var(--t-sheet) var(--ease-out);
  }
  @keyframes toast-in {
    from {
      translate: 0 -12px;
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    @keyframes toast-in {
      from {
        opacity: 0;
      }
    }
  }
</style>
