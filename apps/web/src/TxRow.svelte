<script>
  import { fmtIDR, fmtTime, fmtShort, fmtCur, isFresh } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import Icon from './Icon.svelte'

  /** account: show a transfer as − (out of it) or + (into it) from that account's view */
  let { tx, onclick, showDate = false, account = '' } = $props()
  const fresh = $derived(isFresh(tx.id))
  const look = $derived(catLook(tx.type === 'transfer' ? 'Transfer' : tx.category))
  const into = (n) => n.trim().toLowerCase() === account.trim().toLowerCase()
  const sign = $derived(
    tx.type === 'in' ? '+' : tx.type === 'out' ? '−' : !account ? '' : into(tx.to_account || '') ? '+' : '−',
  )
</script>

<button class="row" class:fresh {onclick}>
  <span class="avatar" style:--hue={look.hue}><Icon name={look.icon} /></span>
  <span class="main">
    <span class="merchant">{tx.merchant || tx.category}</span>
    <span class="meta">
      {tx.category} · {(showDate ? fmtShort : fmtTime).format(new Date(tx.occurred_at))}{tx.account ? ' · ' + tx.account : ''}{tx.to_account ? ' → ' + tx.to_account : ''}
    </span>
    {#if tx.needs_review}<span class="badge">Perlu review</span>{/if}
  </span>
  <span class="amount num {tx.type}">
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
    border-radius: 0;
    border-bottom: 1px solid var(--line);
    color: inherit;
    text-align: left;
    font: inherit;
  }
  .row:last-child {
    border-bottom: 0;
  }
  .main {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
    justify-items: start;
  }
  .merchant {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  .meta {
    max-width: 100%;
    color: var(--muted);
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    margin-top: 2px;
    padding: 1px 8px;
    border-radius: 999px;
    background: var(--warn);
    color: var(--on-warn);
    font-size: 11px;
    font-weight: 600;
  }
  .amount {
    display: grid;
    justify-items: end;
    font-weight: 650;
  }
  small {
    color: var(--muted);
    font-size: 12px;
    font-weight: 400;
  }
  .in {
    color: var(--in);
  }
  .transfer {
    color: var(--muted);
  }
  /* a transaction landing live: an accent wash that settles into the list */
  .fresh {
    animation: landed 1.6s var(--ease-out);
  }
  @keyframes landed {
    from {
      background-color: color-mix(in srgb, var(--accent) 22%, transparent);
      box-shadow:
        -16px 0 0 color-mix(in srgb, var(--accent) 22%, transparent),
        16px 0 0 color-mix(in srgb, var(--accent) 22%, transparent);
    }
  }
  .row:active {
    scale: none;
    background: color-mix(in srgb, var(--text) 5%, transparent);
  }
</style>
