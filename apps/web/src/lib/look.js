// Icon and hue per category / asset kind. Hues are CSS tokens in app.css (--h-*).
const CAT = {
  Makan: ['food', 'orange'],
  Jajan: ['coffee', 'amber'],
  Transport: ['car', 'blue'],
  Belanja: ['bag', 'pink'],
  Tagihan: ['receipt', 'purple'],
  Investasi: ['trend', 'teal'],
  Hiburan: ['play', 'violet'],
  Kesehatan: ['heart', 'red'],
  Gaji: ['briefcase', 'green'],
  'Transfer Masuk': ['transfer', 'green'],
  Refund: ['refund', 'teal'],
  Transfer: ['transfer', 'slate'],
  Penyesuaian: ['scale', 'slate'],
}
export const catLook = (name) => {
  const [icon, hue] = CAT[name] ?? ['grid', 'slate']
  return { icon, hue: `var(--h-${hue})` }
}

const KIND = {
  bank: ['bank', 'blue'], ewallet: ['wallet', 'purple'], forex: ['globe', 'teal'], cash: ['cash', 'green'],
  stock: ['trend', 'orange'], bond: ['doc', 'violet'], sbn: ['doc', 'pink'], mutual_fund: ['pie', 'blue'],
  gold: ['gold', 'amber'], crypto: ['hex', 'orange'], other: ['grid', 'slate'],
}
export const kindLook = (kind) => {
  const [icon, hue] = KIND[kind] ?? KIND.other
  return { icon, hue: `var(--h-${hue})` }
}
