import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Connect } from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { Icon } from './Icon'

export type Connected = { id: string; home: string; name: string; user: string; host: string }

const input = 'field h-8 w-full rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none'
const label = 'mb-1.5 block text-[12px] text-muted'

// サーバーに接続するダイアログ。初めてのサーバーでは、鍵の指紋を見せて確かめてもらう
export function ConnectDialog({ onClose, onConnected }: { onClose: () => void; onConnected: (c: Connected) => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState({ name: '', user: '', port: '22', host: '', auth: 'pass', password: '', keyFile: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [hostKey, setHostKey] = useState<main.HostKey | null>(null)
  const hostRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    hostRef.current?.focus()
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', esc)
    return () => document.removeEventListener('keydown', esc)
  }, [onClose])

  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setF({ ...f, [k]: e.target.value })

  const go = async (trust = '') => {
    const host = f.host.trim(), user = f.user.trim()
    if (!host || !user) return
    setBusy(true)
    setError('')
    const r = await Connect(new main.ConnectRequest({
      host, user, port: parseInt(f.port, 10) || 22,
      password: f.password, keyFile: f.auth === 'key' ? f.keyFile.trim() : '', trust,
    }))
    setBusy(false)
    if (r.hostKey) {
      setHostKey(r.hostKey)
      return
    }
    if (r.error) {
      setError(r.error)
      return
    }
    onConnected({ id: r.id, home: r.home, name: f.name.trim() || host, user, host })
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    go()
  }

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--overlay)] p-4 backdrop-blur-sm" onPointerDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="flex max-h-[calc(100dvh-32px)] w-[min(600px,100%)] flex-col rounded-[14px] border border-line-strong bg-panel shadow-win" role="dialog">
        <div className="flex h-[50px] flex-none items-center border-b border-line pr-2.5 pl-5">
          <h2 className="m-0 flex-1 text-[15px] font-semibold">{t('connect.title')}</h2>
          <button className="grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" onClick={onClose}><Icon name="x" /></button>
        </div>
        <form className="overflow-auto px-5 pt-[18px] pb-5" onSubmit={submit}>
          {hostKey ? <HostKeyCheck hostKey={hostKey} host={f.host} /> : (
            <div className="grid grid-cols-3 gap-3">
              <label><span className={label}>{t('connect.name')}</span><input className={input} value={f.name} onChange={set('name')} /></label>
              <label><span className={label}>{t('connect.user')}</span><input className={input} value={f.user} onChange={set('user')} spellCheck={false} /></label>
              <label><span className={label}>{t('connect.port')}</span><input className={input} value={f.port} onChange={set('port')} inputMode="numeric" /></label>
              <label className="col-span-3"><span className={label}>{t('connect.host')}</span><input ref={hostRef} className={input} value={f.host} onChange={set('host')} spellCheck={false} /></label>
              <label>
                <span className={label}>{t('connect.auth')}</span>
                <select className={input} value={f.auth} onChange={set('auth')}>
                  <option value="pass">{t('connect.authPassword')}</option>
                  <option value="key">{t('connect.authKey')}</option>
                </select>
              </label>
              {f.auth === 'key'
                ? <>
                  <label className="col-span-2"><span className={label}>{t('connect.keyFile')}</span><input className={input} value={f.keyFile} onChange={set('keyFile')} spellCheck={false} /></label>
                  <label className="col-span-3"><span className={label}>{t('connect.passphrase')}</span><input className={input} type="password" value={f.password} onChange={set('password')} /></label>
                </>
                : <label className="col-span-2"><span className={label}>{t('connect.password')}</span><input className={input} type="password" value={f.password} onChange={set('password')} /></label>}
            </div>
          )}
          {error && !hostKey && <p className="mt-4 mb-0 text-[12.5px] break-all text-err">{error}</p>}
          <div className="mt-[18px] flex items-center gap-2">
            {busy && <span className="text-[12px] text-muted">{t('connect.connecting')}</span>}
            <div className="flex-1" />
            <button type="button" className="h-8 rounded-lg border border-line-strong bg-raised px-3.5 hover:bg-btn-hover" onClick={onClose}>{t('connect.cancel')}</button>
            {hostKey
              ? !hostKey.mismatch && <button type="button" className="btn-primary h-8 rounded-lg px-3.5" disabled={busy} onClick={() => go(hostKey.fingerprint)}>{t('connect.trust')}</button>
              : <button type="submit" className="btn-primary h-8 rounded-lg px-3.5" disabled={busy}>{t('connect.submit')}</button>}
          </div>
        </form>
      </div>
    </div>
  )
}

function HostKeyCheck({ hostKey, host }: { hostKey: main.HostKey; host: string }) {
  const { t } = useTranslation()
  return (
    <div className={`rounded-[10px] border p-3.5 ${hostKey.mismatch ? 'border-err' : 'border-line-strong bg-panel-2'}`}>
      <div className={`mb-1.5 flex items-center gap-2 font-semibold ${hostKey.mismatch ? 'text-err' : ''}`}>
        <Icon name="lock" />{hostKey.mismatch ? t('connect.mismatchTitle') : t('connect.newKeyTitle')}
      </div>
      <p className="mt-0 mb-3 text-[12.5px] text-muted">{hostKey.mismatch ? t('connect.mismatchText') : t('connect.newKeyText')}</p>
      <div className="font-mono text-[12px] break-all">{host}</div>
      <div className="font-mono text-[12px] break-all">{hostKey.keyType} {hostKey.fingerprint}</div>
    </div>
  )
}
