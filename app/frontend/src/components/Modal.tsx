import { useEffect } from 'react'
import { useMobile } from '../mobile'
import { Icon } from './Icon'

// 画面の中央に出す窓。Esc と外側のクリックで閉じる
export function Modal({ title, width = 520, height, onClose, children }: {
  title: string
  width?: number
  height?: string
  onClose: () => void
  children: React.ReactNode
}) {
  const mobile = useMobile()
  useEffect(() => {
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', esc)
    return () => document.removeEventListener('keydown', esc)
  }, [onClose])

  return (
    <div className={`fixed inset-0 z-50 grid bg-[var(--overlay)] backdrop-blur-sm ${mobile ? 'items-end' : 'place-items-center p-4'}`} onPointerDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div
        className={`flex flex-col border border-line-strong bg-panel shadow-win ${mobile ? 'max-h-[92dvh] rounded-t-2xl pb-[env(safe-area-inset-bottom)]' : 'max-h-[calc(100dvh-32px)] rounded-[14px]'}`}
        style={{ width: mobile ? '100%' : `min(${width}px, 100%)`, height }}
        role="dialog"
      >
        <div className="flex h-[50px] flex-none items-center border-b border-line pr-2.5 pl-5">
          <h2 className="m-0 flex-1 text-[15px] font-semibold">{title}</h2>
          <button className="grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" onClick={onClose}><Icon name="x" /></button>
        </div>
        <div className="flex min-h-0 flex-1 flex-col overflow-auto px-5 pt-[18px] pb-5">{children}</div>
      </div>
    </div>
  )
}
