<script>
  import { s, fmtIDR, fmtShort, updateTx, addRule } from './lib/store.svelte.js'
  import PageHead from './PageHead.svelte'
  import MonthBar from './MonthBar.svelte'
  import Icon from './Icon.svelte'
  import { catLook } from './lib/look.js'

  let { open } = $props()
  const pending = $derived(s.txs.filter((t) => t.needs_review))
  let remember = $state(true)
  const same = (a, b) => a.trim().toLowerCase() === b.trim().toLowerCase()

  function pick(tx, category) {
    if (!remember || !tx.merchant?.trim()) return updateTx(tx.id, { category, needs_review: false })
    addRule(tx.merchant, tx.type, category)
    // the rule covers future emails; apply it to the pending ones from the same merchant too
    for (const t of pending) if (t.type === tx.type && same(t.merchant || '', tx.merchant)) updateTx(t.id, { category, needs_review: false })
  }
</script>

<PageHead title="Review" />
<MonthBar />

{#if pending.length}
  <p class="hint">Belum dihitung ke saldo aset sampai kamu setujui.</p>
  <label class="remember"><input type="checkbox" bind:checked={remember} /> Ingat pilihan untuk merchant yang sama</label>
{/if}

{#each pending as tx (tx.id)}
  {@const look = catLook(tx.category)}
  <article class="card">
    <button class="head" onclick={() => open(tx)}>
      <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
      <span class="main">
        <b>{tx.merchant || '—'}</b>
        <span>{fmtShort.format(new Date(tx.occurred_at))} · Tebakan: {tx.category}</span>
      </span>
      <b class="num">{fmtIDR.format(tx.amount)}</b>
    </button>
    <div class="chips">
      {#each s.categories.filter((c) => c.type === tx.type) as c}
        <button class:current={c.name === tx.category} onclick={() => pick(tx, c.name)}>
          {c.name}
        </button>
      {/each}
    </div>
  </article>
{:else}
  <section class="card"><p class="empty">Nggak ada yang perlu direview.</p></section>
{/each}

<style>
  .hint {
    color: var(--muted);
    font-size: 13px;
    margin: 0 0 8px;
  }
  .remember {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: 14px;
    margin-bottom: 12px;
  }
  .remember input {
    width: 18px;
    height: 18px;
    margin: 0;
  }
  article {
    margin-bottom: 12px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 12px;
    width: 100%;
    background: none;
    border: 0;
    color: inherit;
    text-align: left;
    font: inherit;
    padding: 0;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
  }
  .main span {
    color: var(--muted);
    font-size: 13px;
  }
  .head:active {
    scale: none;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 12px;
  }
  .chips button {
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text);
    border-radius: 999px;
    padding: 6px 12px;
    font-size: 13px;
  }
  .chips .current {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, var(--surface));
    color: var(--accent);
    font-weight: 600;
  }
</style>
