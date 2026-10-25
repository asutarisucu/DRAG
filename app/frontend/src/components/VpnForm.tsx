import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckVPN } from '../../wailsjs/go/main/App'
import { main, vpn } from '../../wailsjs/go/models'
import { Icon } from './Icon'

// VPN の種類。並びは画面に出す順。ready でないものは選べないようにして出す
export const VPN_TYPES: { type: string; ready: boolean }[] = [
  { type: 'wg', ready: true },
  { type: 'ovpn', ready: false },
  { type: 'ikev2', ready: false },
  { type: 'sstp', ready: false },
  { type: 'ac', ready: false },
  { type: 'l2tp', ready: false },
  { type: 'ts', ready: false },
]

export type VpnCheck = { sum: vpn.Summary | null; error: string }

const input = 'field h-8 rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none'

// 接続の画面で新しい VPN を足す欄。入力は親が持ち、設定を読めたかは check で返す
export function VpnForm({ value, onChange, check, onCheck, configRef }: {
  value: main.VPNInput
  onChange: (v: main.VPNInput) => void
  check: VpnCheck
  onCheck: (c: VpnCheck) => void
  configRef: React.RefObject<HTMLTextAreaElement | null>
}) {
  const { t } = useTranslation()
  const file = useRef<HTMLInputElement>(null)
  const [typing, setTyping] = useState(false)
  const set = (k: keyof main.VPNInput, v: string) => onChange(new main.VPNInput({ ...value, [k]: v }))

  // 打つたびに Go で読み、つなぎ先などを出す
  useEffect(() => {
    if (!value.config.trim()) { onCheck({ sum: null, error: '' }); return }
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

  return (
    <div className="col-span-3 grid gap-2.5 rounded-[10px] border border-line-strong bg-panel-2 p-3" data-vpn-new>
      <div className="flex items-center gap-2">
        <select className={`${input} w-[170px] flex-none`} value={value.type} onChange={e => set('type', e.target.value)} aria-label={t('vpn.type')}>
          {VPN_TYPES.map(x => <option key={x.type} value={x.type} disabled={!x.ready}>{x.ready ? t(`vpn.types.${x.type}`) : t('vpn.soon', { name: t(`vpn.types.${x.type}`) })}</option>)}
        </select>
        <input className={`${input} min-w-0 flex-1`} value={value.name} onChange={e => set('name', e.target.value)} placeholder={t('vpn.name')} spellCheck={false} aria-label={t('vpn.name')} />
      </div>
      <textarea
        ref={configRef}
        className="field min-h-[120px] w-full resize-y rounded-lg border border-line-strong bg-field px-2.5 py-2 font-mono text-[12px] leading-normal whitespace-pre text-text outline-none"
        rows={7} wrap="off" spellCheck={false} value={value.config} onChange={e => set('config', e.target.value)}
        placeholder={t('vpn.configPh')} aria-label={t('vpn.config')}
      />
      <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-2">
        <button type="button" className="flex h-7 flex-none items-center gap-1.5 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover" onClick={() => file.current?.click()}>
          <Icon name="upload" size={14} />{t('vpn.load')}
        </button>
        <input ref={file} type="file" accept=".conf,.txt" hidden onChange={e => { const f = e.target.files?.[0]; if (f) load(f); e.target.value = '' }} />
        {!typing && check.sum && <span className="min-w-0 text-[12px] text-ok" data-vpn-sum>{t('vpn.summary', check.sum)}</span>}
        {!typing && check.error && <span className="min-w-0 text-[12px] text-err" data-vpn-err>{check.error}</span>}
      </div>
      <div className="text-[12px] text-muted">{t('vpn.note')}</div>
    </div>
  )
}
