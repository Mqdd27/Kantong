<script>
  import { s, fmtIDR, fmtShort, updateTx, addRule } from './lib/store.svelte.js'
  import MonthBar from './MonthBar.svelte'

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

<MonthBar />

{#if pending.length}
  <label class="remember"><input type="checkbox" bind:checked={remember} /> Ingat pilihan untuk merchant yang sama</label>
{/if}

{#each pending as tx (tx.id)}
  <article>
    <button class="head" onclick={() => open(tx)}>
      <b>{tx.merchant || '—'}</b>
      <span>{fmtIDR.format(tx.amount)} · {fmtShort.format(new Date(tx.occurred_at))}</span>
      <span class="guess">Tebakan: {tx.category}</span>
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
  <p class="empty">Nggak ada yang perlu direview.</p>
{/each}

<style>
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
    background: var(--surface);
    border-radius: 12px;
    padding: 12px;
    margin-bottom: 12px;
  }
  .head {
    display: grid;
    gap: 2px;
    width: 100%;
    background: none;
    border: 0;
    color: inherit;
    text-align: left;
    font: inherit;
    padding: 0;
  }
  .head span {
    color: var(--muted);
    font-size: 13px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 10px;
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
  }
  .empty {
    color: var(--muted);
  }
</style>
