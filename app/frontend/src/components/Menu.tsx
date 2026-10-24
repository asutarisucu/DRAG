import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useMobile } from '../mobile'
import { Icon, type IconName } from './Icon'

export type MenuItem =
  | { sep: true }
  | { icon: IconName; label: string; hint?: string; disabled?: boolean; danger?: boolean; fn: () => void }

// 右クリックのメニュー。画面の端からはみ出さないように位置をずらす。
// スマホでは画面の下から出し、title (開いた行のパス) を上に書く
export function Menu({ x, y, items, title, onClose }: { x: number; y: number; items: (MenuItem | false | null | undefined)[]; title?: string; onClose: () => void }) {
  const mobile = useMobile()
  const ref = useRef<HTMLDivElement>(null)
  // 長押しで開いたとき、指を離した位置に来た項目が押されてしまう。メニューの中で押し始めたものだけを受ける
  const pressed = useRef(false)
  const [pos, setPos] = useState({ left: x, top: y })

  // 区切りが先頭・末尾・連続にならないように整える
  const list = items.filter((it): it is MenuItem => !!it)
    .filter((it, i, a) => !('sep' in it) || (i > 0 && i < a.length - 1 && !('sep' in a[i + 1])))

  useLayoutEffect(() => {
    if (mobile) return
    const r = ref.current!.getBoundingClientRect()
    setPos({ left: Math.max(8, Math.min(x, innerWidth - r.width - 8)), top: Math.max(8, Math.min(y, innerHeight - r.height - 8)) })
  }, [x, y])

  useEffect(() => {
    const down = (e: PointerEvent) => { if (!ref.current?.contains(e.target as Node)) onClose() }
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('pointerdown', down, true)
    document.addEventListener('keydown', key)
    return () => {
      document.removeEventListener('pointerdown', down, true)
      document.removeEventListener('keydown', key)
    }
  }, [onClose])

  return <>
    {/* スマホでは後ろを暗くする。押すとメニューの外を押したことになって閉じる */}
    {mobile && <div className="anim-fade fixed inset-0 z-[69] bg-black/45" />}
    <div
      ref={ref}
      role="menu"
      className={mobile
        ? 'anim-sheet fixed inset-x-2 bottom-[calc(8px+env(safe-area-inset-bottom))] z-[70] max-h-[80dvh] overflow-y-auto rounded-2xl border border-line-strong bg-raised p-2 shadow-win'
        : 'anim-pop fixed z-[70] min-w-[210px] rounded-[10px] border border-line-strong bg-raised p-[5px] shadow-win'}
      style={mobile ? undefined : pos}
      onPointerDown={() => { pressed.current = true }}
    >
      {mobile && title && <div className="truncate px-3 pt-2 pb-2.5 font-mono text-[13px] text-muted">{title}</div>}
      {list.map((it, i) => 'sep' in it
        ? <div key={i} className="mx-1.5 my-1 h-px bg-line" />
        : (
          <button
            key={i}
            role="menuitem"
            disabled={it.disabled}
            className={`flex w-full items-center gap-2.5 rounded-md px-2.5 text-left enabled:hover:bg-accent-soft disabled:opacity-40 ${mobile ? 'h-12 text-[15px]' : 'h-8 text-[13px]'} ${it.danger ? 'text-err' : ''}`}
            onClick={() => {
              if (!pressed.current) return
              onClose()
              it.fn()
            }}
          >
            <Icon name={it.icon} className={it.danger ? '' : 'text-muted'} />
            <span className="flex-1">{it.label}</span>
            {it.hint && !mobile && <span className="font-mono text-[11px] text-faint">{it.hint}</span>}
          </button>
        ))}
    </div>
  </>
}
