import { useMobile } from '../mobile'
import { dismiss, useToasts } from '../toasts'

export function Toasts() {
  const list = useToasts()
  const mobile = useMobile()
  return (
    <div className={`pointer-events-none fixed z-[60] flex flex-col gap-2 ${mobile ? 'inset-x-3 top-14 items-stretch' : 'right-4 bottom-9 items-end'}`}>
      {list.map(t => (
        <div key={t.id} role="status" className={`toast ${t.kind} pointer-events-auto rounded-[14px] py-[11px] pr-4 pl-[30px] text-[12.5px] ${mobile ? 'anim-toast-m' : 'anim-toast max-w-[380px] min-w-[240px]'} ${t.leaving ? 'toast-out' : ''}`}>
          <div className="flex items-center gap-3">
            <span className="flex-1">{t.text}</span>
            {t.action && (
              <button className="font-medium text-accent-text hover:underline" onClick={() => { t.action!.fn(); dismiss(t.id) }}>{t.action.label}</button>
            )}
          </div>
          {t.progress !== undefined && (
            <div className="mt-[9px] h-[3px] overflow-hidden rounded-sm bg-hover-strong">
              <i className="progress-bar block h-full rounded-sm" style={{ width: `${(t.progress * 100).toFixed(1)}%` }} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
