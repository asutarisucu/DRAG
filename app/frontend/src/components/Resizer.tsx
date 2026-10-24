import { useRef, useState } from 'react'

// 区画の境目。ドラッグで幅か高さを変える。invert は下の区画の高さのように、引くと小さくなる向きのとき
export function Resizer({ dir, value, min, max, onChange, invert = false }: {
  dir: 'v' | 'h'
  value: number
  min: number
  max: number
  onChange: (v: number) => void
  invert?: boolean
}) {
  const start = useRef<{ pos: number; value: number } | null>(null)
  const [active, setActive] = useState(false)
  const pos = (e: React.PointerEvent) => dir === 'v' ? e.clientX : e.clientY

  return (
    <div
      className={`relative z-10 transition-colors hover:bg-accent ${active ? 'bg-accent' : ''} ${dir === 'v' ? 'cursor-col-resize' : 'cursor-row-resize'}`}
      onPointerDown={e => {
        // 幅が変わると押した位置の下にツリーの行が来て、ブラウザーがその行のドラッグを始めてしまう。
        // ここで止めると mousedown が出ず、ドラッグは始まらない
        e.preventDefault()
        e.currentTarget.setPointerCapture(e.pointerId)
        start.current = { pos: pos(e), value }
        setActive(true)
      }}
      onPointerMove={e => {
        if (!start.current) return
        const d = (pos(e) - start.current.pos) * (invert ? -1 : 1)
        onChange(Math.round(Math.min(max, Math.max(min, start.current.value + d))))
      }}
      onPointerUp={() => {
        start.current = null
        setActive(false)
      }}
    />
  )
}
