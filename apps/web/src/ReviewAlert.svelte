<script>
  import { s } from './lib/store.svelte.js'
  import Icon from './Icon.svelte'

  const n = $derived(s.txs.filter((t) => t.needs_review).length)
</script>

{#if n}
  <a class="alert" href="#/review">
    <span class="avatar sm" style:--hue="var(--warn)"><Icon name="inbox" size={18} /></span>
    <span class="txt"><b>{n} transaksi perlu direview</b><span>Belum dihitung ke saldo aset</span></span>
    <Icon name="right" size={18} />
  </a>
{/if}

<style>
  .alert {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--warn) 10%, var(--surface));
    box-shadow: var(--shadow);
    color: var(--text);
    text-decoration: none;
  }
  .txt {
    flex: 1;
    display: grid;
    gap: 1px;
    font-size: 14px;
  }
  .txt span {
    color: var(--muted);
    font-size: 13px;
  }
</style>
