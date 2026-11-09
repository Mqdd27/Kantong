<script>
  let { active } = $props()
</script>

<div class="views" role="tablist">
  <a href="#/tx" role="tab" aria-selected={active === 'tx'} class:on={active === 'tx'}>Daftar</a>
  <a href="#/calendar" role="tab" aria-selected={active === 'calendar'} class:on={active === 'calendar'}>Kalender</a>
</div>

<style>
  .views {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
    padding: 3px;
    border-radius: 10px;
    background: var(--surface);
    margin-bottom: 12px;
  }
  a {
    text-align: center;
    padding: 7px 0;
    border-radius: 8px;
    color: var(--muted);
    text-decoration: none;
    font-size: 14px;
  }
  .on {
    background: var(--bg);
    color: var(--text);
    font-weight: 600;
  }
  @media (min-width: 900px) {
    .views {
      display: none; /* the sidebar has both */
    }
  }
</style>
