<script>
  let { active } = $props()
</script>

<div class="views">
  <a href="#/tx" aria-current={active === 'tx' ? 'page' : undefined}>Daftar</a>
  <a href="#/calendar" aria-current={active === 'calendar' ? 'page' : undefined}>Kalender</a>
</div>

<style>
  .views {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
    padding: 4px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--text) 7%, var(--bg));
    margin-bottom: 12px;
  }
  a {
    text-align: center;
    padding: 8px 0;
    border-radius: 999px;
    color: var(--muted);
    text-decoration: none;
    font-size: 14px;
    font-weight: 500;
  }
  [aria-current='page'] {
    background: var(--surface);
    box-shadow: var(--shadow);
    color: var(--text);
    font-weight: 650;
  }
  @media (min-width: 900px) {
    .views {
      display: none; /* the sidebar has both */
    }
  }
</style>
