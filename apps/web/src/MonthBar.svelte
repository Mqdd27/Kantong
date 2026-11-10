<script>
  import { s, fmtMonth, shiftMonth, setMonth } from './lib/store.svelte.js'
  import Icon from './Icon.svelte'
</script>

<div class="month">
  <button aria-label="Bulan sebelumnya" onclick={() => setMonth(shiftMonth(s.month, -1))}><Icon name="left" /></button>
  <h2>{fmtMonth(s.month)}</h2>
  <button aria-label="Bulan berikutnya" onclick={() => setMonth(shiftMonth(s.month, 1))}><Icon name="right" /></button>
</div>

<style>
  .month {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 12px;
    padding: 2px;
    border-radius: 999px;
    background: var(--surface);
    box-shadow: var(--shadow);
  }
  h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  button {
    display: grid;
    place-items: center;
    width: 44px;
    height: 40px;
    padding: 0;
    border: 0;
    border-radius: 999px;
    background: none;
    color: var(--text);
  }
</style>
