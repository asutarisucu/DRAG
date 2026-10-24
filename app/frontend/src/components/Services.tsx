import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ServiceAction, Services } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { withSudo } from '../sudo'
import { openStreamLog } from '../tools'

const errText = (e: unknown) => String(e).replace(/^Error: /, '')

// 下に数秒だけ出す一言
export function useFoot() {
  const [msg, setMsg] = useState('')
  const timer = useRef(0)
  const say = (m: string) => {
    setMsg(m)
    clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setMsg(''), 2600)
  }
  const foot = <div className={`win-foot ${msg ? 'show' : ''}`} role="status">{msg}</div>
  return [say, foot] as const
}

type Verb = 'start' | 'stop' | 'restart'

// systemd のサービス。動いているものを先に並べ、名前で絞り込む。
// 操作はログインしたユーザーの権限で失敗したら sudo モードを聞く
export function ServicesView({ serverId }: { serverId: string }) {
  const { t } = useTranslation()
  const [list, setList] = useState<remote.Service[] | null>(null)
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  const [busy, setBusy] = useState('')
  const [say, foot] = useFoot()

  const load = () => Services(serverId).then(l => { setList(l ?? []); setError('') }, e => setError(errText(e)))
  useEffect(() => {
    load()
    const iv = setInterval(load, 5000)
    return () => clearInterval(iv)
  }, [serverId])

  const act = (svc: remote.Service, verb: Verb) => {
    setBusy(svc.name)
    withSudo(serverId, t('svc.needSudo'), () => ServiceAction(serverId, svc.name, verb)).then(
      () => say(t('svc.done', { name: svc.name, verb: t(`svc.${verb}`) })),
      e => say(errText(e) === 'permission denied' ? t('files.denied') : errText(e)),
    ).finally(() => {
      setBusy('')
      load()
    })
  }

  if (error && !list) return <div className="tool-empty">{error}</div>
  if (!list) return <div className="tool-empty">{t('mon.waiting')}</div>
  if (!list.length) return <div className="tool-empty">{t('svc.noSystemd')}</div>
  const f = filter.toLowerCase()
  const shown = list
    .filter(s => !f || s.name.toLowerCase().includes(f) || s.desc.toLowerCase().includes(f))
    .sort((a, b) => Number(b.active === 'active') - Number(a.active === 'active') || a.name.localeCompare(b.name))
  return (
    <>
      <div className="win-toolbar">
        <input className="mini-input" placeholder={t('svc.filter')} spellCheck={false} value={filter} onChange={e => setFilter(e.target.value)} />
        <span className="text-[11.5px] text-muted">{t('svc.count', { n: shown.length, all: list.length })}</span>
      </div>
      <div className="svc-list">
        {shown.map(s => {
          const on = s.active === 'active', isBusy = busy === s.name
          const state = isBusy ? t('svc.busy') : s.active === 'failed' ? t('svc.failed') : on ? t('svc.running') : t('svc.stopped')
          return (
            <div key={s.name} className="svc" data-service={s.name}>
              <span className={`svc-dot ${isBusy ? 'busy' : s.active === 'failed' ? 'failed' : on ? 'on' : 'off'}`} />
              <div className="min-w-0">
                <div className="svc-name">{s.name}</div>
                <div className="svc-desc">{s.desc ? `${s.desc} · ${state}` : state}</div>
              </div>
              <div className="svc-actions">
                <button className="sbtn" onClick={() => openStreamLog(serverId, 'journal', s.name)}>{t('svc.log')}</button>
                {on
                  ? <>
                    <button className="sbtn" disabled={isBusy} onClick={() => act(s, 'restart')}>{t('svc.restart')}</button>
                    <button className="sbtn" disabled={isBusy} onClick={() => act(s, 'stop')}>{t('svc.stop')}</button>
                  </>
                  : <button className="sbtn primary" disabled={isBusy} onClick={() => act(s, 'start')}>{t('svc.start')}</button>}
              </div>
            </div>
          )
        })}
      </div>
      {foot}
    </>
  )
}
