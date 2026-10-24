import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { NewPairing, PhoneDevices, PhoneHistory, RevokePhone } from '../../wailsjs/go/main/App'
import type { main, phone } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { toast } from '../toasts'
import { Icon } from './Icon'
import { Modal } from './Modal'

const smallBtn = 'h-7 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover'
const errText = (e: unknown) => String(e).replace(/^Error: /, '')

// 登録した端末とログインの履歴。登録やログインがあると読み直す
export function usePhoneDevices() {
  const [devices, setDevices] = useState<phone.Device[]>([])
  const [history, setHistory] = useState<phone.Login[]>([])
  const load = useCallback(() => {
    PhoneDevices().then(d => setDevices(d ?? []))
    PhoneHistory().then(h => setHistory(h ?? []))
  }, [])
  useEffect(() => {
    load()
    return EventsOn('phone:changed', load)
  }, [load])
  return { devices, history }
}

const pad2 = (n: number) => String(n).padStart(2, '0')
function when(iso: string) {
  const d = new Date(iso)
  const now = new Date()
  const time = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  return d.toDateString() === now.toDateString() ? time : `${d.getMonth() + 1}/${d.getDate()} ${time}`
}

// スマホを登録する窓。QR コードは5分で使えなくなり、1回読まれると次のものに替える
export function PairDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const { devices, history } = usePhoneDevices()
  const [pair, setPair] = useState<main.Pairing | null>(null)
  const [error, setError] = useState('')
  const [now, setNow] = useState(Date.now())

  const created = useRef(0)
  const regen = useCallback(() => {
    created.current = Date.now()
    NewPairing().then(p => { setPair(p); setError('') }, e => setError(errText(e)))
  }, [])
  useEffect(regen, [regen])
  useEffect(() => {
    const iv = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(iv)
  }, [])
  // 読まれたら、同じ QR コードはもう使えないので作り直す
  const paired: string | undefined = history.find(h => h.kind === 'pair')?.time
  useEffect(() => {
    if (!paired || Date.parse(paired) < created.current) return
    toast(t('pair.done'), 'ok')
    regen()
  }, [paired])

  const left = pair ? Math.max(0, Math.floor((pair.expires - now) / 1000)) : 0
  const kindLabel = (k: string) => t(`pair.kinds.${k}`)
  return (
    <Modal title={t('pair.title')} width={640} onClose={onClose}>
      <div className="grid grid-cols-[240px_minmax(0,1fr)] gap-6">
        <div className="grid content-start justify-items-center gap-2 text-center">
          {error
            ? <div className="grid h-[222px] w-[222px] place-items-center rounded-xl border border-line p-4 text-[12.5px] text-err">{error}</div>
            : (
              <svg
                viewBox={`0 0 ${pair?.size ?? 37} ${pair?.size ?? 37}`}
                className={`h-[222px] w-[222px] rounded-xl bg-white transition-[filter,opacity] ${pair && left === 0 ? 'opacity-40 blur-[3px]' : ''}`}
                shapeRendering="crispEdges"
                data-qr={pair?.url ?? ''}
              >
                {pair && <path d={pair.path} fill="#0c0f13" />}
              </svg>
            )}
          <div className="font-mono text-[11.5px] break-all text-muted">{pair ? pair.url.replace(/\/pair\?.*/, '') : ''}</div>
          <div className="text-[12px] text-muted">
            {pair && left > 0 && t('pair.left', { time: `${Math.floor(left / 60)}:${pad2(left % 60)}` })}
            {pair && left === 0 && <>{t('pair.expired')} <button className="text-accent hover:underline" onClick={regen}>{t('pair.regen')}</button></>}
          </div>
        </div>
        <div className="min-w-0">
          <span className="mb-1.5 block text-[12px] text-muted">{t('pair.devices')}</span>
          <div className="mb-3.5 grid gap-1.5">
            {devices.length === 0 && <div className="text-[12px] text-muted">{t('pair.none')}</div>}
            {devices.map(d => (
              <div key={d.id} className="flex items-center gap-2.5 rounded-[10px] border border-line bg-panel-2 px-2.5 py-[9px]" data-device={d.id}>
                <Icon name="phone" className="text-muted" />
                <div className="min-w-0 flex-1">
                  <div className="font-semibold">{d.name}</div>
                  <div className="text-[12px] text-muted">{t('pair.lastLogin', { when: when(d.lastSeen), ip: d.lastIP, route: t(`pair.routes.${d.route}`) })}</div>
                </div>
                <button className={smallBtn} onClick={() => RevokePhone(d.id)}>{t('pair.revoke')}</button>
              </div>
            ))}
          </div>
          <details>
            <summary className="cursor-pointer text-[12px] text-muted">{t('pair.history')}</summary>
            <div className="mt-2 grid max-h-[180px] gap-1 overflow-y-auto text-[12px]">
              {history.length === 0 && <div className="text-muted">{t('pair.none')}</div>}
              {history.map((h, i) => (
                <div key={i} className="grid grid-cols-[90px_1fr_auto_auto] gap-2.5">
                  <span className="font-mono">{when(h.time)}</span>
                  <span className="truncate">{h.device || '—'}</span>
                  <span className="font-mono text-muted">{h.ip}</span>
                  <span className={h.kind === 'denied' || h.kind === 'pair-failed' ? 'text-warn' : 'text-muted'}>{kindLabel(h.kind)}</span>
                </div>
              ))}
            </div>
          </details>
        </div>
      </div>
    </Modal>
  )
}
