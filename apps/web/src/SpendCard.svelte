<script>
  import { s, fmtIDR } from './lib/store.svelte.js'
  import { catLook } from './lib/look.js'
  import Icon from './Icon.svelte'

  /** spent: this month's spending; byCat: [category, amount][] largest first; income: this month's income */
  let { spent = 0, byCat = [], income = 0 } = $props()

  // the card takes on colour as money goes out, measured against the budget, else the month's income
  const limit = $derived(s.budget?.overall || 0)
  // ponytail: no budget and no income yet → half strength once anything is spent
  const ratio = $derived(limit ? s.budget.spent / limit : income ? spent / income : spent ? 0.5 : 0)

  // one colour blob per category, sized by its share of spending
  const SPOTS = [[22, 78], [80, 22], [78, 82], [28, 18], [55, 50], [94, 55], [6, 40]]
  const blobs = $derived.by(() => {
    const rows = byCat.filter(([c]) => c !== 'Investasi').slice(0, SPOTS.length)
    const sum = rows.reduce((n, [, v]) => n + v, 0) || 1
    return rows.map(([c, v], i) => ({ c, hue: catLook(c).hue, size: 45 + (v / sum) * 70, x: SPOTS[i][0], y: SPOTS[i][1] }))
  })
  const net = $derived(s.portfolio.assets.some((a) => a.value_idr < 0))
</script>

<a class="kcard" href="#/analytics" aria-label="Pengeluaran bulan ini {fmtIDR.format(spent)}, {net ? 'kekayaan bersih' : 'total aset'} {fmtIDR.format(s.portfolio.total_idr)}. Buka Analitik">
  <span class="paint" style:opacity={Math.min(1, 0.12 + 0.88 * ratio)} aria-hidden="true">
    {#each blobs as b (b.c)}
      <i style:--hue={b.hue} style:width="{b.size}%" style:left="{b.x}%" style:top="{b.y}%"></i>
    {/each}
  </span>

  <span class="top">
    <img src="favicon.png" alt="" width="34" height="34" />
    <span class="go" aria-hidden="true"><Icon name="right" size={18} /></span>
  </span>
  <span class="spend">
    <span class="lbl">Pengeluaran bulan ini</span>
    <b class="big num">{fmtIDR.format(spent)}</b>
  </span>
  <span class="assets">
    <span class="lbl">{net ? 'Kekayaan bersih' : 'Total aset'}</span>
    <b class="num">{fmtIDR.format(s.portfolio.total_idr)}</b>
  </span>
</a>

<style>
  .kcard {
    position: relative;
    display: grid;
    grid-template-rows: auto 1fr auto;
    gap: 10px;
    aspect-ratio: 1.586; /* a payment card */
    max-height: 360px;
    padding: 18px 20px;
    border-radius: 20px;
    overflow: hidden;
    isolation: isolate;
    background: var(--kcard);
    color: var(--text);
    text-decoration: none;
    box-shadow:
      inset 0 0 0 1px rgb(0 0 0 / 0.05),
      0 10px 28px rgb(0 0 0 / 0.12);
  }
  .paint {
    position: absolute;
    inset: -15%;
    z-index: -1;
    filter: blur(32px);
    transition: opacity var(--t-bar) var(--ease-out);
  }
  .paint i {
    position: absolute;
    aspect-ratio: 1;
    border-radius: 50%;
    translate: -50% -50%;
    background: color-mix(in srgb, var(--hue) 72%, var(--kcard-mix));
  }
  .top {
    display: flex;
    justify-content: space-between;
    align-items: start;
  }
  .top img {
    border-radius: 9px;
  }
  .go {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--kcard) 70%, transparent);
  }
  .spend {
    display: grid;
    align-content: center;
    gap: 2px;
  }
  /* full-strength text: the card behind it can be any colour */
  .lbl {
    font-size: 13px;
    font-weight: 600;
  }
  .big {
    font-size: clamp(28px, 9vw, 38px);
    font-weight: 750;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }
  .assets {
    display: grid;
    gap: 1px;
  }
  .assets b {
    font-size: 17px;
  }
  @media (prefers-reduced-motion: reduce) {
    .paint {
      transition: none;
    }
  }
</style>
