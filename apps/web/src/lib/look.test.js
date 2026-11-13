import { describe, expect, test } from 'vitest'
import { catLook, kindLook } from './look.js'

describe('catLook', () => {
  test('known category', () => expect(catLook('Makan')).toEqual({ icon: 'food', hue: 'var(--h-orange)' }))
  test('multi-word category', () => expect(catLook('Transfer Masuk')).toEqual({ icon: 'transfer', hue: 'var(--h-green)' }))
  test('unknown falls back to grid/slate', () => expect(catLook('Nope')).toEqual({ icon: 'grid', hue: 'var(--h-slate)' }))
  test('undefined falls back', () => expect(catLook(undefined)).toEqual({ icon: 'grid', hue: 'var(--h-slate)' }))
})

describe('kindLook', () => {
  test('known kind', () => expect(kindLook('gold')).toEqual({ icon: 'gold', hue: 'var(--h-amber)' }))
  test('unknown falls back to other', () => expect(kindLook('nft')).toEqual({ icon: 'grid', hue: 'var(--h-slate)' }))
})
