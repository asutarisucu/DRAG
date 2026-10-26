import { useLayoutEffect, useRef } from 'react'

// 細かい動き。mock/v2/motion.js と同じ動きを、React の部品から使う形にしたもの
const SPRING = 'cubic-bezier(.3,1.3,.5,1)'
const still = () => matchMedia('(prefers-reduced-motion: reduce)').matches

type Move = (el: HTMLElement, from: DOMRect, to: DOMRect) => void

// 選んでいる項目の下地 (::before など) を、前に選んでいた項目の位置から滑らせる。
// 項目には data-slide に key を入れておく。前の項目がまだあれば、いまの位置から動かす
export function useSlide(box: React.RefObject<HTMLElement | null>, key: string | undefined, move: Move) {
  const last = useRef<{ key: string; rect: DOMRect } | null>(null)
  useLayoutEffect(() => {
    const find = (k: string) => box.current?.querySelector<HTMLElement>(`[data-slide="${CSS.escape(k)}"]`)
    const el = key ? find(key) : null
    if (!key || !el) {
      last.current = null
      return
    }
    const rect = el.getBoundingClientRect()
    const prev = last.current
    if (prev && prev.key !== key && rect.width && !still()) move(el, find(prev.key)?.getBoundingClientRect() ?? prev.rect, rect)
    last.current = { key, rect }
  }, [key])
}

// 横に並んだ項目の下地を滑らせる
export const slideX = (pseudo: string): Move => (el, from, to) => el.animate(
  [{ transform: `translateX(${from.left - to.left}px) scaleX(${from.width / to.width})` }, { transform: 'none' }],
  { duration: 420, easing: SPRING, pseudoElement: pseudo })

// 横に並んだ項目の、下の真ん中の印を滑らせる
export const slideMark: Move = (el, from, to) => {
  const dx = from.left + from.width / 2 - (to.left + to.width / 2)
  el.animate([{ transform: `translateX(-50%) translateX(${dx}px)` }, { transform: 'translateX(-50%)' }], { duration: 420, easing: SPRING, pseudoElement: '::after' })
}

// 縦に並んだ項目の、左の印を滑らせる
export const slideY: Move = (el, from, to) => el.animate(
  [{ transform: `translateY(${from.top - to.top}px)`, opacity: 1 }, { transform: 'none', opacity: 1 }],
  { duration: 420, easing: SPRING, pseudoElement: '::before' })

// 初めて画面に出た要素を動かす。ref に渡す。描き直しや、隠していた区画を出したときは動かさない。
// ツリーの行は --i の順にずらして、上から順に出す
export function enterRow(el: HTMLElement | null) {
  if (!el || still()) return
  const i = Number(el.style.getPropertyValue('--i')) || 0
  el.animate([{ opacity: 0, transform: 'translateX(-8px)' }, { opacity: 1, transform: 'none' }],
    { duration: 340, delay: Math.min(i, 20) * 18, easing: 'cubic-bezier(.2,.8,.2,1)', fill: 'backwards' })
}

export function enterTask(el: HTMLElement | null) {
  if (!el || still()) return
  el.animate([{ opacity: 0, transform: 'translateY(10px) scale(.86)' }, { opacity: 1, transform: 'none' }], { duration: 450, easing: 'cubic-bezier(.34,1.5,.64,1)' })
}

// ウィンドウはぼかしから浮かび上がらせる
export function enterWindow(el: HTMLElement) {
  if (still()) return
  el.animate([
    { opacity: 0, transform: 'translateY(18px) scale(.94)', filter: 'blur(6px)' },
    { opacity: 1, transform: 'none', filter: 'blur(0)' },
  ], { duration: 520, easing: 'cubic-bezier(.2,1.15,.35,1)' })
}

// ガラスの板は、ポインターの位置に光を当てる。位置は CSS の --mx と --my で受け取る
document.addEventListener('pointermove', e => {
  const card = (e.target as Element | null)?.closest?.<HTMLElement>('button.glass-card')
  if (!card) return
  const r = card.getBoundingClientRect()
  card.style.setProperty('--mx', `${e.clientX - r.left}px`)
  card.style.setProperty('--my', `${e.clientY - r.top}px`)
}, { passive: true })
