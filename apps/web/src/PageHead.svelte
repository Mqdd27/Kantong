<script>
  import { s, view, toggleHide } from './lib/store.svelte.js'
  import Icon from './Icon.svelte'

  /** title: page name; children: extra icon buttons before the eye toggle */
  let { title, children } = $props()
</script>

<header>
  <h1>{title}</h1>
  <div class="act">
    {@render children?.()}
    <button class="icon" aria-label={view.hide ? 'Tampilkan nominal' : 'Sembunyikan nominal'} aria-pressed={view.hide} onclick={toggleHide}>
      <Icon name={view.hide ? 'eyeoff' : 'eye'} />
    </button>
    <span class="dot" class:on={s.live} role="img" aria-label={s.live ? 'Realtime tersambung' : 'Menyambung…'} title={s.live ? 'Realtime tersambung' : 'Menyambung…'}></span>
  </div>
</header>

<style>
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
    min-height: 44px;
  }
  h1 {
    margin: 0;
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .act {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  /* shared look for every icon button placed in the header */
  .act :global(.icon) {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: var(--surface);
    box-shadow: var(--shadow);
    color: var(--text);
    text-decoration: none;
  }
  .dot {
    width: 8px;
    height: 8px;
    margin-left: 6px;
    border-radius: 50%;
    background: var(--muted);
    transition: background-color var(--t-sheet);
  }
  .dot.on {
    background: var(--in);
  }
</style>
