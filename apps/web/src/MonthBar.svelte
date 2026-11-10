<script>
  import { s, fmtMonth, shiftMonth, setMonth } from './lib/store.svelte.js'
</script>

<header>
  <button aria-label="Bulan sebelumnya" onclick={() => setMonth(shiftMonth(s.month, -1))}>‹</button>
  <h2>{fmtMonth(s.month)}</h2>
  <button aria-label="Bulan berikutnya" onclick={() => setMonth(shiftMonth(s.month, 1))}>›</button>
  <span class="dot" class:on={s.live} role="img" aria-label={s.live ? 'Realtime tersambung' : 'Menyambung…'} title={s.live ? 'Realtime tersambung' : 'Menyambung…'}></span>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
  }
  h2 {
    flex: 1;
    text-align: center;
    margin: 0;
    font-size: 18px;
  }
  button {
    background: none;
    border: 0;
    color: var(--text);
    font-size: 26px;
    padding: 0;
    min-width: 44px;
    min-height: 44px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--muted);
    transition: background-color var(--t-sheet);
  }
  .dot.on {
    background: var(--in);
  }
</style>
