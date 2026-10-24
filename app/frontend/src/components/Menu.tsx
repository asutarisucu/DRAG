import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Icon, type IconName } from './Icon'

export type MenuItem =
  | { sep: true }
  | { icon: IconName; label: string; hint?: string; disabled?: boolean; danger?: boolean; fn: () => void }

// 右クリックのメニュー。画面の端からはみ出さないように位置をずらす
export function Menu({ x, y, items, onClose }: { x: number; y: number; items: (MenuItem | false | null | undefined)[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: x, top: y })

  // 区切りが先頭・末尾・連続にならないように整える
  const list = items.filter((it): it is MenuItem => !!it)
    .filter((it, i, a) => !('sep' in it) || (i > 0 && i < a.length - 1 && !('sep' in a[i + 1])))

  useLayoutEffect(() => {
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

  return (
    <div ref={ref} role="menu" className="fixed z-[70] min-w-[210px] rounded-[10px] border border-line-strong bg-raised p-[5px] shadow-win" style={pos}>
      {list.map((it, i) => 'sep' in it
        ? <div key={i} className="mx-1.5 my-1 h-px bg-line" />
        : (
          <button
            key={i}
            role="menuitem"
            disabled={it.disabled}
            className={`flex h-8 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] enabled:hover:bg-accent-soft disabled:opacity-40 ${it.danger ? 'text-err' : ''}`}
            onClick={() => { onClose(); it.fn() }}
          >
            <Icon name={it.icon} className={it.danger ? '' : 'text-muted'} />
            <span className="flex-1">{it.label}</span>
            {it.hint && <span className="font-mono text-[11px] text-faint">{it.hint}</span>}
          </button>
        ))}
    </div>
  )
}
