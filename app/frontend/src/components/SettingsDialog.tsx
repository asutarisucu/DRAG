import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { PhoneState, SetPhoneLAN, SetPhonePublic } from '../../wailsjs/go/main/App'
import type { main } from '../../wailsjs/go/models'
import { useMobile } from '../mobile'
import { isRemote } from '../remote'
import { DEFAULTS, setSettings, useSettings } from '../settings'
import { Icon, type IconName } from './Icon'
import { Modal } from './Modal'

function Seg<T extends string>({ value, options, onPick }: { value: T; options: [T, string][]; onPick: (v: T) => void }) {
  return (
    <div className="flex gap-0.5 rounded-lg border border-line bg-field p-0.5">
      {options.map(([v, label]) => (
        <button key={v} className={`h-6 rounded-md px-2.5 text-[12px] whitespace-nowrap ${v === value ? 'bg-raised text-text shadow-[0_0_0_1px_var(--border-strong)]' : 'text-muted hover:text-text'}`} onClick={() => onPick(v)}>{label}</button>
      ))}
    </div>
  )
}

function Row({ label, desc, children }: { label: string; desc?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-[18px] border-b border-line py-[13px]">
      <div>
        <div>{label}</div>
        {desc && <div className="mt-0.5 text-[11.5px] text-muted">{desc}</div>}
      </div>
      {children}
    </div>
  )
}

function Switch({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={on}
      className={`relative h-[22px] w-[38px] flex-none rounded-full transition-colors ${on ? 'bg-accent' : 'bg-line-strong'}`}
      onClick={() => onChange(!on)}
    >
      <span className={`absolute top-[3px] left-[3px] h-4 w-4 rounded-full bg-white transition-transform ${on ? 'translate-x-4' : ''}`} />
    </button>
  )
}

// 自動でウィンドウに移すコマンドの一覧。× で外し、入力欄で足す
function AutoCmds({ list, onChange }: { list: string[]; onChange: (l: string[]) => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState('')
  return (
    <div className="border-b border-line pt-1 pb-3.5">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {list.map(c => (
          <span key={c} className="inline-flex h-6 items-center gap-1 rounded-full border border-line-strong bg-panel-2 pr-1 pl-2.5 font-mono text-[12px]">
            {c}
            <button className="grid h-4 w-4 place-items-center rounded-full text-muted hover:text-text" title={t('settings.autoRemove')} onClick={() => onChange(list.filter(x => x !== c))}>
              <Icon name="x" size={11} />
            </button>
          </span>
        ))}
      </div>
      <input
        className="field h-8 w-full rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none"
        placeholder={t('settings.autoAdd')}
        spellCheck={false}
        value={v}
        onChange={e => setV(e.target.value)}
        onKeyDown={e => {
          const c = v.trim().replace(/\s+/g, ' ')
          if (e.key === 'Enter' && c && !list.includes(c)) {
            onChange([...list, c])
            setV('')
          }
        }}
      />
    </div>
  )
}

type Page = 'look' | 'term' | 'ops' | 'phone'

// スマホの待ち受け。PC の設定フォルダに置き、Go の側で切り替える
function PhonePage() {
  const { t } = useTranslation()
  const [st, setSt] = useState<main.PhoneState | null>(null)
  const [error, setError] = useState('')
  const [port, setPort] = useState('')
  // 証明書を取り終えたら、期限を出し直す
  useEffect(() => {
    const load = () => PhoneState().then(s => { setSt(s); setPort(p => p || String(s.settings.publicPort)) })
    load()
    return EventsOn('phone:changed', load)
  }, [])
  if (!st) return null
  const done = (s: main.PhoneState) => { setSt(s); setError('') }
  const failed = (e: unknown) => { setError(String(e)); PhoneState().then(setSt) }
  const desc = [st.status.url || `:${st.settings.port}`, t('settings.selfSigned')].join(' · ')
  const pub = st.public
  const until = pub.certUntil ? new Date(pub.certUntil) : null
  const certDesc = !st.settings.public ? t('settings.certOff')
    : pub.error ? pub.error
      : pub.pending || !until ? t('settings.certPending')
        : t('settings.certIP', { url: pub.url, until: `${until.getMonth() + 1}/${until.getDate()}` })
  const applyPort = () => {
    const n = Number(port)
    if (n === st.settings.publicPort) return
    SetPhonePublic(st.settings.public, n).then(done, e => { failed(e); setPort(String(st.settings.publicPort)) })
  }
  return (
    <>
      <Row label={t('settings.phoneLan')} desc={error || st.status.error || desc}>
        <Switch on={st.settings.lan} onChange={on => SetPhoneLAN(on).then(done, failed)} />
      </Row>
      <Row label={t('settings.phonePublic')} desc={t('settings.phonePublicDesc')}>
        <Switch on={st.settings.public} onChange={on => SetPhonePublic(on, Number(port) || st.settings.publicPort).then(done, failed)} />
      </Row>
      <Row label={t('settings.publicPort')} desc={t('settings.publicPortDesc')}>
        <input
          className="field h-8 w-[100px] rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none"
          inputMode="numeric"
          value={port}
          onChange={e => setPort(e.target.value.replace(/\D/g, ''))}
          onBlur={applyPort}
          onKeyDown={e => { if (e.key === 'Enter') applyPort() }}
        />
      </Row>
      <Row label={t('settings.cert')} desc={certDesc}>
        <span className="text-[12px] text-muted">{t('settings.certKind')}</span>
      </Row>
    </>
  )
}

// 設定。ページを替えても文字の大きさを変えても、窓の高さは変えない
export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const s = useSettings()
  const mobile = useMobile()
  const [page, setPage] = useState<Page>('look')
  // スマホの待ち受けは PC アプリの設定なので、スマホの画面には出さない
  const pages: [Page, IconName][] = [['look', 'window'], ['term', 'terminal'], ['ops', 'gear'], ...(isRemote ? [] : [['phone', 'phone'] as [Page, IconName]])]

  return (
    <Modal title={t('settings.title')} width={780} height={mobile ? 'calc(100dvh - 56px)' : undefined} onClose={onClose}>
      <div className={mobile ? 'grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-3' : 'grid h-[440px] flex-none grid-cols-[160px_minmax(0,1fr)] gap-6'}>
        <div className={`flex gap-0.5 ${mobile ? 'flex-row flex-wrap' : 'flex-col'}`}>
          {pages.map(([k, icon]) => (
            <button key={k} className={`flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-left ${page === k ? 'bg-accent-soft text-text' : 'text-muted hover:text-text'}`} onClick={() => setPage(k)}>
              <Icon name={icon} />{t(`settings.${k}`)}
            </button>
          ))}
          {!mobile && <div className="flex-1" />}
          <button className="h-7 self-center rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover" onClick={() => setSettings({ lang: DEFAULTS.lang, theme: DEFAULTS.theme, termSize: DEFAULTS.termSize, uploadConflict: DEFAULTS.uploadConflict, autoOn: DEFAULTS.autoOn, autoCmds: DEFAULTS.autoCmds, snap: DEFAULTS.snap, followLog: DEFAULTS.followLog, sb: DEFAULTS.sb })}>
            {t('settings.reset')}
          </button>
        </div>
        <div className="min-h-0 overflow-y-auto">
          <h3 className="mt-0 mb-1 text-[15px]">{t(`settings.${page}`)}</h3>
          {page === 'look' && <>
            <Row label={t('settings.lang')}>
              <Seg value={s.lang} options={[['ja', '日本語'], ['en', 'English']]} onPick={lang => setSettings({ lang })} />
            </Row>
            <Row label={t('settings.theme')}>
              <Seg value={s.theme} options={[['dark', t('settings.dark')], ['light', t('settings.light')]]} onPick={theme => setSettings({ theme })} />
            </Row>
            {!isRemote && (
              <Row label={t('settings.statusItems')}>
                <div className="flex flex-wrap justify-end gap-x-3.5 gap-y-1">
                  {(['uptime', 'cpu', 'mem', 'net'] as const).map(k => (
                    <label key={k} className="inline-flex cursor-pointer items-center gap-1.5 select-none">
                      <input type="checkbox" className="accent-[var(--accent)]" checked={s.sb[k]} onChange={e => setSettings({ sb: { ...s.sb, [k]: e.target.checked } })} />{t(`settings.sb.${k}`)}
                    </label>
                  ))}
                </div>
              </Row>
            )}
          </>}
          {page === 'ops' && <>
            <Row label={t('settings.autoOn')} desc={t('settings.autoOnDesc')}>
              <Switch on={s.autoOn} onChange={autoOn => setSettings({ autoOn })} />
            </Row>
            <AutoCmds list={s.autoCmds} onChange={autoCmds => setSettings({ autoCmds })} />
            <Row label={t('settings.snap')} desc={t('settings.snapDesc')}>
              <Switch on={s.snap} onChange={snap => setSettings({ snap })} />
            </Row>
            <Row label={t('settings.followLog')}>
              <Switch on={s.followLog} onChange={followLog => setSettings({ followLog })} />
            </Row>
            <Row label={t('settings.uploadConflict')}>
              <select
                className="field h-8 min-w-[140px] rounded-lg border border-line-strong bg-field px-2.5 text-text outline-none"
                value={s.uploadConflict}
                onChange={e => setSettings({ uploadConflict: e.target.value as typeof s.uploadConflict })}
              >
                <option value="ask">{t('settings.conflictAsk')}</option>
                <option value="overwrite">{t('upload.overwrite')}</option>
                <option value="keep">{t('upload.keep')}</option>
                <option value="skip">{t('upload.skip')}</option>
              </select>
            </Row>
          </>}
          {page === 'phone' && <PhonePage />}
          {page === 'term' && (
            <Row label={t('settings.termSize')}>
              <div className="flex items-center gap-2.5">
                <input type="range" min={11} max={18} step={1} value={s.termSize} className="w-40 accent-[var(--accent)]" onChange={e => setSettings({ termSize: +e.target.value })} />
                <span className="font-mono text-[11.5px]">{s.termSize}px</span>
              </div>
            </Row>
          )}
        </div>
      </div>
    </Modal>
  )
}
