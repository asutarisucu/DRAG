import { useMobile } from '../mobile'
import { dismiss, useToasts } from '../toasts'

const edge = { info: 'border-l-accent', ok: 'border-l-ok', warn: 'border-l-warn' }

export function Toasts() {
  const list = useToasts()
  const mobile = useMobile()
  return (
    <div className={`pointer-events-none fixed z-[60] flex flex-col gap-2 ${mobile ? 'inset-x-3 top-14 items-stretch' : 'right-4 bottom-9 items-end'}`}>
      {list.map(t => (
        <div key={t.id} role="status" className={`pointer-events-auto rounded-[10px] ${mobile ? '' : 'max-w-[380px] min-w-[240px]'} border border-l-[3px] border-line-strong bg-[color-mix(in_srgb,var(--raised)_96%,transparent)] px-3.5 py-2.5 text-[12.5px] shadow-win backdrop-blur ${edge[t.kind]}`}>
          <div className="flex items-center gap-3">
            <span className="flex-1">{t.text}</span>
            {t.action && (
              <button className="font-semibold text-accent" onClick={() => { t.action!.fn(); dismiss(t.id) }}>{t.action.label}</button>
            )}
          </div>
          {t.progress !== undefined && (
            <div className="mt-2 h-1 overflow-hidden rounded-sm bg-line">
              <i className="block h-full bg-gradient-to-r from-accent to-accent-2" style={{ width: `${(t.progress * 100).toFixed(1)}%` }} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
