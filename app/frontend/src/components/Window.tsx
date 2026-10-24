import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useSettings } from '../settings'
import { focusWin, updateWin, type Rect, type Win } from '../windows'
import { Icon } from './Icon'

export type SnapMode = 'left' | 'right' | 'max' | null

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

// デスクトップのウィンドウの枠。バーをドラッグして動かし、右と下の縁で大きさを変える。
// 画面の左右の端へ寄せると半分、上の端で最大化 (mock/ と同じ)
export function Window({ win, focused, host, onSnapPreview, onMinimize, onClose, children }: {
  win: Win
  focused: boolean
  host: () => DOMRect
  onSnapPreview: (m: SnapMode) => void
  onMinimize: () => void
  onClose: () => void
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  const settings = useSettings()
  const el = useRef<HTMLDivElement>(null)

  const toggleMax = () => win.max
    ? updateWin(win.id, { max: false, rect: win.prev ?? win.rect })
    : updateWin(win.id, { max: true, snapped: false, prev: win.snapped ? win.prev : win.rect })

  const onBarDown = (e: React.PointerEvent) => {
    if (e.button !== 0 || (e.target as HTMLElement).closest('button')) return
    e.preventDefault()
    const bar = e.currentTarget as HTMLElement
    bar.setPointerCapture(e.pointerId)
    const H = host()
    let start = { px: e.clientX, py: e.clientY, ...win.rect }
    let moved = false, mode: SnapMode = null
    const move = (ev: PointerEvent) => {
      if (!moved) {
        if (Math.hypot(ev.clientX - start.px, ev.clientY - start.py) < 4) return
        moved = true
        if (win.max || win.snapped) {
          // 最大化や半分のまま掴んだら、元の大きさに戻してから動かす
          const prev = win.prev ?? win.rect
          const ratio = clamp((ev.clientX - H.left - win.rect.x) / win.rect.w, 0, 1)
          start = { px: ev.clientX, py: ev.clientY, x: ev.clientX - H.left - prev.w * ratio, y: Math.max(0, ev.clientY - H.top - 16), w: prev.w, h: prev.h }
          updateWin(win.id, { max: false, snapped: false, rect: { x: start.x, y: start.y, w: start.w, h: start.h } })
        }
      }
      const x = clamp(start.x + ev.clientX - start.px, -start.w + 90, H.width - 90)
      const y = clamp(start.y + ev.clientY - start.py, 0, H.height - 34)
      updateWin(win.id, { rect: { x, y, w: start.w, h: start.h } })
      mode = !settings.snap ? null
        : ev.clientX <= H.left + 8 ? 'left' : ev.clientX >= H.right - 8 ? 'right' : ev.clientY <= H.top + 6 ? 'max' : null
      onSnapPreview(mode)
    }
    const up = () => {
      bar.removeEventListener('pointermove', move)
      bar.removeEventListener('pointerup', up)
      bar.removeEventListener('pointercancel', up)
      onSnapPreview(null)
      if (!moved) return
      const before: Rect = { x: start.x, y: start.y, w: start.w, h: start.h }
      if (mode === 'max') updateWin(win.id, { max: true, prev: before })
      else if (mode) updateWin(win.id, { snapped: true, prev: before, rect: { x: mode === 'left' ? 0 : H.width / 2, y: 0, w: H.width / 2, h: H.height } })
    }
    bar.addEventListener('pointermove', move)
    bar.addEventListener('pointerup', up)
    bar.addEventListener('pointercancel', up)
  }

  const onResizeDown = (dx: boolean, dy: boolean) => (e: React.PointerEvent) => {
    if (e.button !== 0 || win.max) return
    e.preventDefault()
    e.stopPropagation()
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    const s = { px: e.clientX, py: e.clientY, ...win.rect }
    const move = (ev: PointerEvent) => updateWin(win.id, {
      snapped: false,
      rect: { x: s.x, y: s.y, w: dx ? Math.max(260, s.w + ev.clientX - s.px) : s.w, h: dy ? Math.max(150, s.h + ev.clientY - s.py) : s.h },
    })
    const up = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', up)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', up)
  }

  const r = win.max ? { x: 0, y: 0, w: undefined, h: undefined } : win.rect
  const btn = 'grid h-[26px] w-8 place-items-center rounded-md text-muted hover:bg-raised hover:text-text'
  return (
    <div
      ref={el}
      data-win={win.id}
      className={`absolute flex min-h-[150px] min-w-[260px] flex-col overflow-hidden border bg-[var(--win-bg)] shadow-win ${win.max ? 'inset-0 rounded-none' : 'rounded-[10px]'} ${focused ? 'border-[color-mix(in_srgb,var(--accent)_50%,transparent)]' : 'border-line-strong'} ${win.min ? 'hidden' : ''}`}
      style={{ left: r.x, top: r.y, width: r.w, height: r.h, zIndex: win.z }}
      onPointerDownCapture={() => { if (!focused) focusWin(win.id) }}
    >
      <div
        className={`flex h-[34px] flex-none touch-none items-center gap-2 border-b border-line pr-1 pl-2.5 select-none ${focused ? 'bg-gradient-to-b from-[var(--bar-f1)] to-[var(--bar-f2)]' : 'bg-gradient-to-b from-[var(--bar-1)] to-[var(--bar-2)]'}`}
        onPointerDown={onBarDown}
        onDoubleClick={e => { if (!(e.target as HTMLElement).closest('button')) toggleMax() }}
      >
        <Icon name={win.icon} className={focused ? 'text-accent' : 'text-muted'} />
        <span className={`min-w-0 flex-1 truncate text-[12.5px] ${win.ended ? 'text-faint' : ''}`}>{win.title}</span>
        <div className="flex">
          <button className={btn} title={t('desktop.minimize')} onClick={onMinimize}><Icon name="min" size={14} /></button>
          <button className={btn} title={win.max ? t('desktop.restore') : t('desktop.maximize')} onClick={toggleMax}><Icon name={win.max ? 'restore' : 'max'} size={14} /></button>
          <button className={`${btn} hover:!bg-[#c42b1c] hover:!text-white`} title={t('desktop.close')} onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
      </div>
      <div className="relative flex min-h-0 flex-1 flex-col">{children}</div>
      {!win.max && <>
        <div className="absolute top-0 -right-[3px] bottom-0 z-[3] w-2 cursor-ew-resize" onPointerDown={onResizeDown(true, false)} />
        <div className="absolute right-0 -bottom-[3px] left-0 z-[3] h-2 cursor-ns-resize" onPointerDown={onResizeDown(false, true)} />
        <div className="absolute -right-[3px] -bottom-[3px] z-[3] h-4 w-4 cursor-nwse-resize" onPointerDown={onResizeDown(true, true)} />
      </>}
    </div>
  )
}
