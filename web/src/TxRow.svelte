<script>
  import { fmtIDR, fmtTime, fmtShort, fmtCur } from './lib/store.svelte.js'

  let { tx, onclick, showDate = false } = $props()
  const sign = $derived(tx.type === 'in' ? '+' : tx.type === 'out' ? '−' : '')
</script>

<button class="row" {onclick}>
  <span class="main">
    <span class="merchant">{tx.merchant || tx.category}</span>
    <span class="meta">
      {tx.category} · {(showDate ? fmtShort : fmtTime).format(new Date(tx.occurred_at))}{tx.account ? ' · ' + tx.account : ''}
      {#if tx.needs_review}<span class="badge">review</span>{/if}
    </span>
  </span>
  <span class="amount {tx.type}">
    {sign}{fmtIDR.format(tx.amount)}
    {#if tx.currency && tx.currency !== 'IDR'}<small>{fmtCur(tx.orig_amount, tx.currency)}</small>{/if}
  </span>
</button>

<style>
  .row {
    display: flex;
    width: 100%;
    gap: 12px;
    align-items: center;
    padding: 12px 0;
    background: none;
    border: 0;
    border-bottom: 1px solid var(--line);
    color: inherit;
    text-align: left;
    font: inherit;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
  }
  .merchant {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    color: var(--muted);
    font-size: 13px;
  }
  .badge {
    background: var(--warn);
    color: #fff;
    border-radius: 4px;
    padding: 0 5px;
    font-size: 11px;
    margin-left: 4px;
  }
  .amount {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    display: grid;
    justify-items: end;
  }
  small {
    color: var(--muted);
    font-size: 12px;
  }
  .in {
    color: var(--in);
  }
  .transfer {
    color: var(--muted);
  }
</style>
