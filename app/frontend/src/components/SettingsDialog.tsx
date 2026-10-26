import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { DeleteVPN, PhoneState, SetPhoneLAN, SetPhonePublic, VPNs } from '../../wailsjs/go/main/App'
import type { main } from '../../wailsjs/go/models'
import { useMobile } from '../mobile'
import { isRemote } from '../remote'
import { DEFAULTS, setSettings, useSettings } from '../settings'
import { Icon, type IconName } from './Icon'
import { Menu } from './Menu'
import { Modal } from './Modal'
import { VpnEdit } from './VpnForm'

function Seg<T extends string>({ value, options, onPick }: { value: T; options: [T, string][]; onPick: (v: T) => void }) {
  return (
    <div className="seg">
      {options.map(([v, label]) => (
        <button key={v} className={v === value ? 'active' : ''} onClick={() => onPick(v)}>{label}</button>
      ))}
    </div>
  )
}

// アクセントの色の候補。mock/v2/index.html の MOCK_THEME.accents と同じ。紫は参考にした AuthKit のアクセントの色
const ACCENTS: [string, string][] = [['#0a6cff', 'blue'], ['#027dea', 'cyan'], ['#663af3', 'purple'], ['#269684', 'green'], ['#e46d4c', 'orange'], ['#ec4f86', 'pink']]

function Accent({ value, onPick }: { value: string; onPick: (c: string) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-center justify-end gap-1.5">
      {ACCENTS.map(([c, name]) => (
        <button
          key={c}
          title={t(`settings.accents.${name}`)}
          data-accent={c}
          className={`swatch ${value.toLowerCase() === c ? 'active' : ''}`}
          style={{ background: c }}
          onClick={() => onPick(c)}
        />
      ))}
      <input type="color" className="swatch-custom" title={t('settings.accentCustom')} value={value} onChange={e => onPick(e.target.value)} />
    </div>
  )
}

function Row({ label, desc, children }: { label: string; desc?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-[18px] border-b border-line-soft py-3.5">
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
    <button role="switch" aria-checked={on} className="switch" onClick={() => onChange(!on)} />
  )
}

// 自動でウィンドウに移すコマンドの一覧。× で外し、入力欄で足す
function AutoCmds({ list, onChange }: { list: string[]; onChange: (l: string[]) => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState('')
  return (
    <div className="border-b border-line-soft pt-1 pb-3.5">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {list.map(c => (
          <span key={c} className="anim-pop inline-flex h-7 items-center gap-1 rounded-full bg-hover pr-1 pl-3 font-mono text-[12px] text-strong shadow-[var(--edge)]">
            {c}
            <button className="icon-btn h-5 w-5 hover:!text-err" title={t('settings.autoRemove')} onClick={() => onChange(list.filter(x => x !== c))}>
              <Icon name="x" size={11} />
            </button>
          </span>
        ))}
      </div>
      <input
        className="field h-[34px] w-full px-2.5"
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

type Page = 'look' | 'term' | 'ops' | 'phone' | 'vpn'

// 保存した VPN。足すのはサーバーに接続するときで、ここでは見て消す
// 保存した VPN。右クリックで編集と削除ができる。この VPN でつないでいるサーバーがあれば、どちらもさせない
function VpnPage() {
  const { t } = useTranslation()
  const [list, setList] = useState<main.VPNInfo[] | null>(null)
  const [error, setError] = useState('')
  const [menu, setMenu] = useState<{ x: number; y: number; vpn: main.VPNInfo } | null>(null)
  const [editing, setEditing] = useState<main.VPNInfo | null>(null)
  const load = () => VPNs().then(l => setList(l ?? []))
  useEffect(() => { load() }, [])
  if (!list) return null
  if (list.length === 0) return <div className="py-[13px] text-[12px] text-muted">{t('vpn.empty')}</div>
  const remove = (v: main.VPNInfo) => DeleteVPN(v.id).then(() => { setError(''); load() }, e => setError(String(e)))
  return (
    <>
      {error && <div className="py-2 text-[12px] text-err">{error}</div>}
      {list.map(v => (
        <div key={v.id} onContextMenu={e => { e.preventDefault(); setMenu({ x: e.clientX, y: e.clientY, vpn: v }) }} data-vpn-row={v.name}>
          <Row label={v.name} desc={[`${t(`vpn.types.${v.type}`)} · ${v.summary.target}`, v.servers.length > 0 && t('vpn.usedBy', { names: v.servers.join(', ') })].filter(Boolean).join(' · ')}>
            <button
              className="btn sm"
              disabled={v.inUse}
              title={v.inUse ? t('vpn.inUse') : undefined}
              onClick={() => remove(v)}
              data-vpn-remove={v.name}
            >{t('vpn.remove')}</button>
          </Row>
        </div>
      ))}
      {menu && <Menu x={menu.x} y={menu.y} title={menu.vpn.name} onClose={() => setMenu(null)} items={[
        { icon: 'edit', label: t('vpn.edit'), disabled: menu.vpn.inUse, fn: () => setEditing(menu.vpn) },
        { sep: true },
        { icon: 'trash', label: t('vpn.remove'), danger: true, disabled: menu.vpn.inUse, fn: () => remove(menu.vpn) },
      ]} />}
      {editing && <VpnEdit vpn={editing} onClose={() => setEditing(null)} onSaved={() => { setError(''); load() }} />}
    </>
  )
}

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
          className="field h-[34px] w-[100px] px-2.5"
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
  // スマホの待ち受けと保存した VPN は PC アプリの設定なので、スマホの画面には出さない
  const pages: [Page, IconName][] = [['look', 'window'], ['term', 'terminal'], ['ops', 'gear'], ...(isRemote ? [] : [['phone', 'phone'], ['vpn', 'shield']] as [Page, IconName][])]

  return (
    <Modal title={t('settings.title')} width={780} height={mobile ? 'calc(100dvh - 56px)' : undefined} onClose={onClose}>
      <div className={mobile ? 'grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-3' : 'grid h-[440px] flex-none grid-cols-[170px_minmax(0,1fr)] gap-6'}>
        <div className={`flex gap-0.5 ${mobile ? 'flex-row flex-wrap' : 'flex-col'}`}>
          {pages.map(([k, icon]) => (
            <button key={k} className={`flex h-9 items-center gap-2.5 rounded-full px-3 text-left transition-colors ${page === k ? 'bg-hover-strong font-medium text-strong shadow-[var(--edge)]' : 'text-muted hover:bg-hover hover:text-strong'}`} onClick={() => setPage(k)}>
              <Icon name={icon} />{t(`settings.${k}`)}
            </button>
          ))}
          {!mobile && <div className="flex-1" />}
          <button className="btn sm self-center" onClick={() => setSettings({ lang: DEFAULTS.lang, theme: DEFAULTS.theme, accent: DEFAULTS.accent, desk: DEFAULTS.desk, term: DEFAULTS.term, termSize: DEFAULTS.termSize, uploadConflict: DEFAULTS.uploadConflict, autoOn: DEFAULTS.autoOn, autoCmds: DEFAULTS.autoCmds, snap: DEFAULTS.snap, followLog: DEFAULTS.followLog, sb: DEFAULTS.sb })}>
            {t('settings.reset')}
          </button>
        </div>
        <div className="min-h-0 overflow-y-auto">
          <h3 className="mt-0 mb-1 text-[16px] font-semibold text-strong">{t(`settings.${page}`)}</h3>
          {page === 'look' && <>
            <Row label={t('settings.lang')}>
              <Seg value={s.lang} options={[['ja', '日本語'], ['en', 'English']]} onPick={lang => setSettings({ lang })} />
            </Row>
            <Row label={t('settings.theme')}>
              <Seg value={s.theme} options={[['dark', t('settings.dark')], ['light', t('settings.light')]]} onPick={theme => setSettings({ theme })} />
            </Row>
            <Row label={t('settings.accent')} desc={t('settings.accentDesc')}>
              <Accent value={s.accent} onPick={accent => setSettings({ accent })} />
            </Row>
            <Row label={t('settings.desk')}>
              <Seg value={s.desk} options={[['gradient', t('settings.deskGradient')], ['dots', t('settings.deskDots')], ['plain', t('settings.deskPlain')]]} onPick={desk => setSettings({ desk })} />
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
                className="field h-[34px] min-w-[140px] px-2.5"
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
          {page === 'vpn' && <VpnPage />}
          {page === 'term' && <>
            <Row label={t('settings.termScheme')}>
              <Seg value={s.term} options={[['dark', t('settings.dark')], ['light', t('settings.light')], ['solarized', 'Solarized']]} onPick={term => setSettings({ term })} />
            </Row>
            <Row label={t('settings.termSize')}>
              <div className="flex items-center gap-2.5">
                <input type="range" min={11} max={18} step={1} value={s.termSize} className="w-40 accent-[var(--accent)]" onChange={e => setSettings({ termSize: +e.target.value })} />
                <span className="font-mono text-[11.5px]">{s.termSize}px</span>
              </div>
            </Row>
            {/* 配色の見本。色は style.css の --term-* */}
            <div className="mt-3.5 rounded-xl bg-term px-3.5 py-3 font-mono leading-[1.5] whitespace-pre-wrap text-term-fg shadow-[inset_0_0_0_1px_var(--term-line)]" style={{ fontSize: s.termSize }} data-term-preview>
              <div><span className="text-term-prompt">user@server:~$ </span><span className="text-term-strong">ls</span></div>
              <div><span className="font-semibold text-term-dir">backups</span>  <span className="font-semibold text-term-dir">mc</span>  notes.txt  <span className="text-term-hl">mc-2026-10-24.tar.gz</span></div>
              <div><span className="text-term-dim">[09:41:07 WARN]: </span><span className="text-term-warn">Can't keep up! Is the server overloaded?</span></div>
              <div className="text-term-err">foo: command not found</div>
            </div>
          </>}
        </div>
      </div>
    </Modal>
  )
}
