import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckVPN, TailscaleLogin, TailscaleWait } from '../../wailsjs/go/main/App'
import { main, vpn } from '../../wailsjs/go/models'
import { Icon } from './Icon'

// VPN の種類。並びは画面に出す順。
// file は設定ファイルをそのまま使う種類で、そうでない種類はサーバーとユーザー名とパスワードで入る。
// Tailscale だけは、ブラウザでログインして DRAG を tailnet の端末の1つにする
export const VPN_TYPES: { type: string; file: boolean }[] = [
  { type: 'wg', file: true },
  { type: 'ovpn', file: true },
  { type: 'ikev2', file: false },
  { type: 'sstp', file: false },
  { type: 'ac', file: false },
  { type: 'l2tp', file: false },
  { type: 'ts', file: false },
]

export type VpnCheck = { sum: vpn.Summary | null; error: string }

const input = 'field h-8 rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none'
const label = 'mb-1.5 block text-[12px] text-muted'
const smallBtn = 'flex h-7 flex-none items-center gap-1.5 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover'

// pem は、読み込んだ証明書を PEM にする。DER (.cer) なら base64 にして包む
async function pem(f: File): Promise<string> {
  const b = new Uint8Array(await f.arrayBuffer())
  const text = new TextDecoder().decode(b)
  if (text.includes('-----BEGIN')) return text
  let s = ''
  b.forEach(x => { s += String.fromCharCode(x) })
  return `-----BEGIN CERTIFICATE-----\n${btoa(s).replace(/.{64}/g, '$&\n')}\n-----END CERTIFICATE-----\n`
}

// 接続の画面で新しい VPN を足す欄。入力は親が持ち、設定を読めたかは check で返す
export function VpnForm({ value, onChange, check, onCheck, configRef, userRef }: {
  value: main.VPNInput
  onChange: (v: main.VPNInput) => void
  check: VpnCheck
  onCheck: (c: VpnCheck) => void
  configRef: React.RefObject<HTMLTextAreaElement | null>
  userRef: React.RefObject<HTMLInputElement | null>
}) {
  const { t } = useTranslation()
  const file = useRef<HTMLInputElement>(null)
  const caFile = useRef<HTMLInputElement>(null)
  const [typing, setTyping] = useState(false)
  const [caName, setCaName] = useState('')
  const [ts, setTs] = useState({ busy: false, ok: false, text: '' })
  const kind = VPN_TYPES.find(x => x.type === value.type) ?? VPN_TYPES[0]
  const set = (k: keyof main.VPNInput, v: string) => {
    onChange(new main.VPNInput({ ...value, [k]: v }))
    // 足りないと出したあとに打ったら、その知らせを消す
    if (k !== 'config' && check.error) onCheck({ ...check, error: '' })
  }

  // 設定ファイルは、打つたびに Go で読み、つなぎ先などを出す
  useEffect(() => {
    if (!kind.file || !value.config.trim()) { onCheck({ sum: null, error: '' }); return }
    setTyping(true)
    const id = setTimeout(() => {
      CheckVPN(value).then(sum => onCheck({ sum, error: '' }), err => onCheck({ sum: null, error: String(err) })).finally(() => setTyping(false))
    }, 200)
    return () => clearTimeout(id)
  }, [value.type, value.config])

  const load = async (f: File) => {
    const config = await f.text()
    onChange(new main.VPNInput({ ...value, config, name: value.name.trim() ? value.name : f.name.replace(/\.[^.]+$/, '') }))
  }
  const loadCA = async (f: File) => {
    set('ca', await pem(f))
    setCaName(f.name)
  }

  // ログインは Go がブラウザで開き、済むまで待つ。config には Go が持っているログインの番号を入れる
  const tsLogin = async () => {
    setTs({ busy: true, ok: false, text: t('vpn.tsWait') })
    try {
      const l = await TailscaleLogin()
      const info = await TailscaleWait(l.id)
      onChange(new main.VPNInput({ ...value, config: l.id, name: value.name.trim() ? value.name : info.tailnet.split('.')[0] }))
      if (check.error) onCheck({ ...check, error: '' })
      setTs({ busy: false, ok: true, text: t('vpn.tsDone', { tailnet: info.tailnet, node: info.node }) })
    } catch (err) {
      setTs({ busy: false, ok: false, text: String(err) })
    }
  }

  const login = (
    <div className="grid grid-cols-2 gap-x-3">
      <label><span className={label}>{t('vpn.user')}</span><input ref={userRef} className={`${input} w-full`} value={value.user} onChange={e => set('user', e.target.value)} spellCheck={false} /></label>
      <label><span className={label}>{t('vpn.password')}</span><input className={`${input} w-full`} type="password" value={value.password} onChange={e => set('password', e.target.value)} /></label>
    </div>
  )
  const error = !typing && check.error && <span className="min-w-0 text-[12px] text-err" data-vpn-err>{check.error}</span>

  return (
    <div className="col-span-3 grid gap-2.5 rounded-[10px] border border-line-strong bg-panel-2 p-3" data-vpn-new>
      <div className="flex items-center gap-2">
        <select className={`${input} w-[170px] flex-none`} value={value.type} onChange={e => set('type', e.target.value)} aria-label={t('vpn.type')}>
          {VPN_TYPES.map(x => <option key={x.type} value={x.type}>{t(`vpn.types.${x.type}`)}</option>)}
        </select>
        <input className={`${input} min-w-0 flex-1`} value={value.name} onChange={e => set('name', e.target.value)} placeholder={t('vpn.name')} spellCheck={false} aria-label={t('vpn.name')} />
      </div>
      {value.type === 'ts'
        ? <>
          <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-2" data-vpn-ts>
            <button type="button" className={smallBtn} disabled={ts.busy} onClick={tsLogin}>{t('vpn.tsLogin')}</button>
            {ts.text && <span className={`min-w-0 text-[12px] ${ts.ok ? 'text-ok' : ts.busy ? 'text-muted' : 'text-err'}`}>{ts.text}</span>}
          </div>
          {error}
        </>
        : kind.file
        ? <>
          <textarea
            ref={configRef}
            className="field min-h-[120px] w-full resize-y rounded-lg border border-line-strong bg-field px-2.5 py-2 font-mono text-[12px] leading-normal whitespace-pre text-text outline-none"
            rows={7} wrap="off" spellCheck={false} value={value.config} onChange={e => set('config', e.target.value)}
            placeholder={t(value.type === 'ovpn' ? 'vpn.configPhOvpn' : 'vpn.configPh')} aria-label={t('vpn.config')}
          />
          <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-2">
            <button type="button" className={smallBtn} onClick={() => file.current?.click()}>
              <Icon name="upload" size={14} />{t('vpn.load')}
            </button>
            <input ref={file} type="file" accept=".conf,.ovpn,.txt" hidden onChange={e => { const f = e.target.files?.[0]; if (f) load(f); e.target.value = '' }} />
            {!typing && check.sum && (
              <span className="min-w-0 text-[12px] text-ok" data-vpn-sum>
                {/* OpenVPN は、この PC のアドレスと通す範囲をサーバーが張るときに決める */}
                {check.sum.address ? t('vpn.summary', { target: check.sum.target, address: check.sum.address, routes: check.sum.routes }) : t('vpn.summaryTarget', { target: check.sum.target })}
                {check.sum.login && ` · ${t('vpn.login')}`}
              </span>
            )}
            {error}
          </div>
          {check.sum?.login && login}
        </>
        : <>
          <div className="grid grid-cols-2 gap-x-3">
            <label className={value.type === 'sstp' ? 'col-span-2' : ''}><span className={label}>{t('vpn.server')}</span><input className={`${input} w-full`} value={value.server} onChange={e => set('server', e.target.value)} placeholder="vpn.example.com" spellCheck={false} /></label>
            {/* IKEv2 は証明書でも入れる。いまはユーザー名とパスワードだけ */}
            {value.type === 'ikev2' && (
              <label>
                <span className={label}>{t('vpn.auth')}</span>
                <select className={`${input} w-full`} value="eap" onChange={() => {}}>
                  <option value="eap">{t('vpn.authPassword')}</option>
                  <option value="cert" disabled>{t('vpn.soon', { name: t('vpn.authCert') })}</option>
                </select>
              </label>
            )}
            {value.type === 'ac' && (
              <label><span className={label}>{t('vpn.group')}</span><input className={`${input} w-full`} value={value.group} onChange={e => set('group', e.target.value)} placeholder={t('vpn.groupPh')} spellCheck={false} /></label>
            )}
            {value.type === 'l2tp' && (
              <label><span className={label}>{t('vpn.psk')}</span><input className={`${input} w-full`} type="password" value={value.psk} onChange={e => set('psk', e.target.value)} /></label>
            )}
          </div>
          {login}
          {/* L2TP/IPsec は事前共有キーで確かめるので、サーバーの証明書を見ない */}
          {value.type !== 'l2tp' && (
            <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-2">
              <button type="button" className={smallBtn} onClick={() => caFile.current?.click()}>
                <Icon name="upload" size={14} />{t('vpn.loadCA')}
              </button>
              <input ref={caFile} type="file" accept=".crt,.pem,.cer" hidden onChange={e => { const f = e.target.files?.[0]; if (f) loadCA(f); e.target.value = '' }} />
              {value.ca
                ? <span className="flex min-w-0 items-center gap-2 text-[12px] text-ok" data-vpn-ca>{t('vpn.caLoaded', { name: caName || 'CA' })}<button type="button" className="text-muted hover:text-text" onClick={() => { set('ca', ''); setCaName('') }}>{t('vpn.caClear')}</button></span>
                : <span className="min-w-0 text-[12px] text-muted">{t('vpn.caNone')}</span>}
            </div>
          )}
          {error}
        </>}
      <div className="text-[12px] text-muted">{t('vpn.note')}</div>
    </div>
  )
}
