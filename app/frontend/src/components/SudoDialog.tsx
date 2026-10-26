import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EnableSudo } from '../../wailsjs/go/main/App'
import { setSudo, useSudoAsk } from '../sudo'
import { Modal } from './Modal'

const input = 'field h-[34px] w-full px-2.5'
const btn = 'btn'

// sudo モードを ON にするときにパスワードを聞く。パスワードが違うと分かるまで sudo は2秒ほどかかる
export function SudoDialog({ who }: { who: (serverId: string) => string }) {
  const ask = useSudoAsk()
  if (!ask) return null
  return <SudoForm key={ask.serverId + ask.reason} who={who(ask.serverId)} serverId={ask.serverId} reason={ask.reason} resolve={ask.resolve} />
}

function SudoForm({ who, serverId, reason, resolve }: { who: string; serverId: string; reason: string; resolve: (ok: boolean) => void }) {
  const { t } = useTranslation()
  const [pass, setPass] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    setError('')
    EnableSudo(serverId, pass).then(() => {
      setSudo(serverId, true)
      resolve(true)
    }, err => {
      setBusy(false)
      setError(String(err).replace(/^Error: /, ''))
    })
  }
  return (
    <Modal title={t('sudo.title')} width={420} onClose={() => { if (!busy) resolve(false) }}>
      <form onSubmit={submit}>
        {reason && <p className="mt-0 mb-3.5">{reason}</p>}
        <label>
          <span className="mb-1.5 block text-[12px] font-medium text-soft">{t('sudo.password', { who })}</span>
          <input className={input} type="password" autoFocus value={pass} disabled={busy} onChange={e => setPass(e.target.value)} />
        </label>
        <div className={`mt-2 min-h-[18px] text-[12px] ${error ? 'text-err' : 'text-muted'}`}>{error || (busy ? t('sudo.checking') : '')}</div>
        <div className="mt-3.5 flex items-center gap-2">
          <div className="flex-1" />
          <button type="button" className={btn} disabled={busy} onClick={() => resolve(false)}>{t('connect.cancel')}</button>
          <button type="submit" className="btn-primary" disabled={busy}>{t('sudo.turnOn')}</button>
        </div>
      </form>
    </Modal>
  )
}
