<script>
  import { s, fmtIDR, fmtShort, updateTx } from './lib/store.svelte.js'
  import MonthBar from './MonthBar.svelte'

  let { open } = $props()
  const pending = $derived(s.txs.filter((t) => t.needs_review))
</script>

<MonthBar />

{#each pending as tx (tx.id)}
  <article>
    <button class="head" onclick={() => open(tx)}>
      <b>{tx.merchant || '—'}</b>
      <span>{fmtIDR.format(tx.amount)} · {fmtShort.format(new Date(tx.occurred_at))}</span>
      <span class="guess">Tebakan: {tx.category}</span>
    </button>
    <div class="chips">
      {#each s.categories.filter((c) => c.type === tx.type) as c}
        <button class:current={c.name === tx.category} onclick={() => updateTx(tx.id, { category: c.name, needs_review: false })}>
          {c.name}
        </button>
      {/each}
    </div>
  </article>
{:else}
  <p class="empty">Nggak ada yang perlu direview.</p>
{/each}

<style>
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
