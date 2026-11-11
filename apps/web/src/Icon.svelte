<script>
  // 24px line icons, one stroke weight. Decorative by default; pass label when the icon stands alone.
  let { name, size = 20, label = '' } = $props()

  const P = {
    home: 'M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1z',
    list: 'M9 6h12M9 12h12M9 18h12M4 6h.01M4 12h.01M4 18h.01',
    plus: 'M12 5v14M5 12h14',
    chat: 'M4 5h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9l-5 4V6a1 1 0 0 1 1-1zM8 10h.01M12 10h.01M16 10h.01',
    send: 'M4 12 20 4l-6 16-3-7zM11 13l9-9',
    pie: 'M21 12A9 9 0 1 1 12 3v9zM15 3.5A9 9 0 0 1 20.5 9H15z',
    chart: 'M3 20h18M6 16v-5M11 16V6M16 16v-8M21 16v-3',
    wallet: 'M19 7V5H5a2 2 0 0 0 0 4h15v11H5a2 2 0 0 1-2-2V7M16.5 14.5h.01',
    calendar: 'M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v13a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2zM4 10h16M8 2v4M16 2v4',
    inbox: 'M3 13h5l1.5 3h5L16 13h5M5.5 5h13L21 13v6a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-6z',
    tag: 'M3 3h8l10 10-8 8L3 11zM7.5 7.5h.01',
    download: 'M12 3v12M7 10l5 5 5-5M4 21h16',
    left: 'M15 18l-6-6 6-6',
    right: 'M9 6l6 6-6 6',
    search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4-4',
    eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
    eyeoff: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM3 3l18 18',
    alert: 'M12 9v4M12 17h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z',
    up: 'M12 19V5M6 11l6-6 6 6',
    down: 'M12 5v14M6 13l6 6 6-6',
    repeat: 'M17 2l4 4-4 4M3 11V9a3 3 0 0 1 3-3h15M7 22l-4-4 4-4M21 13v2a3 3 0 0 1-3 3H3',
    // categories
    food: 'M4 3v7a3 3 0 0 0 6 0V3M7 3v18M17 21V3c-2.5 1-4 3.5-4 7v3h4',
    coffee: 'M4 8h13v5a6 6 0 0 1-6 6h-1a6 6 0 0 1-6-6zM17 9h1.5a2.5 2.5 0 0 1 0 5H17M8 2v3M12 2v3',
    car: 'M5 17h14v-5l-2-5H7l-2 5zM5 12h14M7 17v2M17 17v2M7.5 14.5h.01M16.5 14.5h.01',
    bag: 'M6 7h12l1 14H5zM9 7a3 3 0 0 1 6 0',
    receipt: 'M6 2h12v20l-3-2-3 2-3-2-3 2zM9 7h6M9 11h6M9 15h4',
    trend: 'M3 17l6-6 4 4 8-8M15 7h6v6',
    play: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM10 8.5v7l6-3.5z',
    heart: 'M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z',
    briefcase: 'M3 8h18v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1zM8 8V5a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v3M3 13h18',
    refund: 'M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5',
    transfer: 'M7 4 3 8l4 4M3 8h14M17 20l4-4-4-4M21 16H7',
    scale: 'M12 3v18M8 21h8M5 7h14M5 7l-3 6a3 3 0 0 0 6 0zM19 7l-3 6a3 3 0 0 0 6 0z',
    grid: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
    // asset kinds
    bank: 'M3 21h18M4 10h16M12 3l9 5H3zM6 10v8M10 10v8M14 10v8M18 10v8',
    globe: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3 12h18M12 3c2.5 2.5 3.5 5.5 3.5 9s-1 6.5-3.5 9c-2.5-2.5-3.5-5.5-3.5-9s1-6.5 3.5-9z',
    cash: 'M2 6h20v12H2zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 12h.01M18 12h.01',
    doc: 'M7 3h10l4 4v14H7zM17 3v4h4M10 12h8M10 16h8',
    gold: 'M3 19l2.5-7h13L21 19zM7.5 12 9.5 6h5l2 6',
    hex: 'M12 2l9 5v10l-9 5-9-5V7z',
  }
</script>

<svg
  viewBox="0 0 24 24"
  width={size}
  height={size}
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden={label ? undefined : 'true'}
  role={label ? 'img' : undefined}
  aria-label={label || undefined}
><path d={P[name] ?? P.grid} /></svg>

<style>
  svg {
    display: block;
    flex: none;
  }
</style>
