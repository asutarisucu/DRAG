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
    {mobile && <div className="anim-fade fixed inset-0 z-[69] bg-[var(--overlay)] backdrop-blur-[3px]" />}
    <div
      ref={ref}
      role="menu"
      className={mobile
        ? 'menu-card anim-sheet fixed inset-x-2 bottom-[calc(8px+env(safe-area-inset-bottom))] z-[70] max-h-[80dvh] overflow-y-auto rounded-[20px] p-2'
        : 'menu-card anim-pop fixed z-[70] min-w-[220px] rounded-[14px] p-1.5'}
      style={mobile ? undefined : pos}
      onPointerDown={() => { pressed.current = true }}
    >
      {mobile && title && <div className="truncate px-3 pt-2 pb-2.5 font-mono text-[13px] text-muted">{title}</div>}
      {/* 項目は上から順に出す。--i はその順番 */}
      {list.map((it, i) => 'sep' in it
        ? <div key={i} className="menu-sep mx-1.5 my-1 h-px" style={{ ['--i' as string]: Math.min(i, 14) }} />
        : (
          <button
            key={i}
            role="menuitem"
            disabled={it.disabled}
            style={{ ['--i' as string]: Math.min(i, 14) }}
            className={`menu-item flex w-full items-center gap-2.5 px-2.5 text-left transition-colors disabled:opacity-40 ${mobile ? 'h-12 rounded-xl text-[15px]' : 'h-8 rounded-lg text-[13px]'} ${it.danger ? 'text-err enabled:hover:bg-[color-mix(in_srgb,var(--err)_14%,transparent)]' : 'enabled:hover:bg-hover-strong enabled:hover:text-strong'}`}
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
