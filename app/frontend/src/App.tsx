import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Disconnect, PhoneState, Reconnect, SavedServers, Servers, SetLanguage } from '../wailsjs/go/main/App'
import type { store } from '../wailsjs/go/models'
import { EventsOn, OnFileDrop, OnFileDropOff, WindowHide, WindowIsMaximised, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime/runtime'
import { ConnectDialog, type Connected } from './components/ConnectDialog'
import { Desktop } from './components/Desktop'
import { FileTree } from './components/FileTree'
import { PairDialog, usePhoneDevices } from './components/PairDialog'
import { Icon, type IconName } from './components/Icon'
import { Resizer } from './components/Resizer'
import { SessionsView } from './components/SessionsView'
import { openSessionWindow } from './openSession'
import { sessionKeyOf } from './sessions'
import { SettingsDialog } from './components/SettingsDialog'
import { SudoDialog } from './components/SudoDialog'
import { TerminalPanel } from './components/TerminalPanel'
import { Toasts } from './components/Toasts'
import { setSettings, useSettings } from './settings'
import { setMTab, useMobile, useMTab, type MTab } from './mobile'
import { fmtGB, fmtRate, fmtUptime, pct, useMetrics } from './metrics'
import { Spark } from './components/Spark'
import { openTool } from './tools'
import { isRemote, onLink } from './remote'
import { requireSudo, setSudo, sudoOn, turnOffSudo, useSudo } from './sudo'
import { toast } from './toasts'
import iconUrl from './assets/icon-64.png'
import logoDark from './assets/logo-dark.png'
import logoLight from './assets/logo-light.png'

// ファイルの区画の最小の幅。見出しの「ファイル」と5つのボタンが1行に入る幅
const SIDEBAR_MIN = 200

// conn は接続の様子。切れたら retry 回目の再接続まで wait 秒待つ。
// gen は再接続のたびに増え、ターミナルとファイルツリーが開き直す
type Server = Connected & {
  conn: 'ok' | 'lost' | 'reconnecting'
  retry: number
  wait: number
  gen: number
  error: string
}

// 全体の枠。上がサーバーのタブ、左がファイルツリー、右上がデスクトップ、右下がターミナル
export default function App() {
  const { t } = useTranslation()
  const [servers, setServers] = useState<Server[]>([])
  const [active, setActive] = useState('')
  const [connecting, setConnecting] = useState<{ preset?: store.Server } | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [pairOpen, setPairOpen] = useState(false)
  // スマホの待ち受けが止まっていれば、登録の窓の代わりに設定を案内する
  const openPair = () => PhoneState().then(s => s.status.running || s.public.running ? setPairOpen(true) : toast(t('pair.off'), 'warn'))
  const serversRef = useRef(servers)
  serversRef.current = servers
  const update = (id: string, f: (s: Server) => Partial<Server>) => setServers(list => list.map(s => s.id === id ? { ...s, ...f(s) } : s))

  useEffect(() => EventsOn('server:closed', (id: string) => {
    update(id, s => s.conn === 'ok' ? { conn: 'lost', retry: 1, wait: 3, error: '' } : {})
  }), [])

  // 接続は PC とスマホで共有する。ほかの端末がつないだサーバーはタブを足し、切ったものは外す
  const addServer = (c: Connected, activate: boolean) => {
    setServers(list => list.some(s => s.id === c.id) ? list : [...list, { ...c, conn: 'ok', retry: 0, wait: 0, gen: 0, error: '' }])
    setActive(a => activate || !a ? c.id : a)
  }
  useEffect(() => EventsOn('server:opened', (c: Connected) => addServer(c, false)), [])
  // AI エージェントがスマホの登録 URL を出したら知らせる。出したことはスマホの履歴にも残る
  useEffect(() => EventsOn('agent:pair', (route: string) => toast(t('pair.agentIssued', { route: t(`pair.routes.${route}`) }), 'warn')), [t])
  useEffect(() => EventsOn('server:removed', (id: string) => {
    setServers(list => {
      const rest = list.filter(s => s.id !== id)
      setActive(a => a === id ? rest.at(-1)?.id ?? '' : a)
      return rest
    })
  }), [])
  // 通知領域のメニューを画面の言語に合わせる
  const lang = useSettings().lang
  useEffect(() => { if (!isRemote) SetLanguage(lang) }, [lang])

  // スマホの画面は、開いたときに PC アプリがつないでいるサーバーを並べる
  const [linkLost, setLinkLost] = useState(false)
  useEffect(() => {
    if (!isRemote) return
    Servers().then(list => list.forEach(c => addServer(c as Connected, false)))
    if (new URLSearchParams(location.search).has('paired')) {
      toast(t('pair.registered'), 'ok')
      history.replaceState(null, '', '/')
    }
    return onLink(l => setLinkLost(l === 'lost'))
  }, [])

  // PC のファイルのドロップ。落ちた位置の行からフォルダを決め、そのサーバーのファイルツリーに渡す
  useEffect(() => {
    OnFileDrop((x, y, paths) => {
      const target = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-drop-dir]')
      const server = target?.closest<HTMLElement>('[data-server]')
      if (!target || !server) return
      window.dispatchEvent(new CustomEvent('drag:file-drop', { detail: { serverId: server.dataset.server, dir: target.dataset.dropDir, paths } }))
    }, false)
    return () => OnFileDropOff()
  }, [])

  // 切れているサーバーの待ち時間を1秒ずつ減らし、0 になったらつなぎ直す
  const waiting = servers.some(s => s.conn === 'lost' && !s.error)
  useEffect(() => {
    if (!waiting) return
    const iv = setInterval(() => setServers(list => list.map(s => s.conn === 'lost' && !s.error && s.wait > 0 ? { ...s, wait: s.wait - 1 } : s)), 1000)
    return () => clearInterval(iv)
  }, [waiting])

  const reconnect = useCallback((id: string) => {
    update(id, () => ({ conn: 'reconnecting' }))
    Reconnect(id).then(r => {
      if (r.hostKey) update(id, () => ({ conn: 'lost', error: r.error }))
      else if (r.error) update(id, s => ({ conn: 'lost', retry: s.retry + 1, wait: 5 }))
      else {
        update(id, s => ({ conn: 'ok', retry: 0, gen: s.gen + 1, home: r.home }))
        toast(t('conn.reconnected', { name: serversRef.current.find(s => s.id === id)?.name ?? '' }), 'ok')
        if (sudoOn(id) && !r.sudo) toast(t('sudo.lost'), 'warn')
        setSudo(id, r.sudo)
      }
    })
  }, [t])

  useEffect(() => {
    for (const s of servers) if (s.conn === 'lost' && !s.error && s.wait <= 0) reconnect(s.id)
  }, [servers, reconnect])

  const added = (c: Connected) => {
    addServer(c, true)
    setConnecting(null)
  }
  // スマホではタブを閉じるだけにする。接続は PC アプリのもので、PC の画面でも使っている
  const close = (id: string) => {
    if (!isRemote) {
      Disconnect(id)
      toast(t('conn.disconnected', { name: servers.find(s => s.id === id)?.name ?? '' }))
    }
    setSudo(id, false)
    setServers(list => {
      const rest = list.filter(s => s.id !== id)
      setActive(a => a === id ? rest.at(-1)?.id ?? '' : a)
      return rest
    })
  }
  const cur = servers.find(s => s.id === active)
  const sudo = useSudo(active)
  // スマホの幅では、上はいま見ているサーバーの名前だけにし、下の欄の代わりに区画を切り替えるタブを置く
  const mobile = useMobile()
  const mtab = useMTab()
  const rows = mobile ? (cur ? 'grid-rows-[48px_minmax(0,1fr)_60px]' : 'grid-rows-[48px_minmax(0,1fr)]') : 'grid-rows-[42px_minmax(0,1fr)_24px]'

  return (
    <div className={`grid h-dvh ${rows}`}>
      <header
        className={`app-bar flex min-w-0 items-center gap-2 border-b border-line bg-chrome select-none ${mobile ? 'px-2.5' : 'px-2'} ${sudo ? 'shadow-[inset_0_-2px_0_var(--warn)]' : ''}`}
        onDoubleClick={e => { if (!isRemote && !mobile && !(e.target as HTMLElement).closest('button, .no-drag')) WindowToggleMaximise() }}
      >
        <img src={iconUrl} alt="DRAG" draggable={false} className="mr-1 ml-1 h-[22px] w-[22px] flex-none" />
        <div className="flex min-w-0 gap-1 overflow-x-auto [scrollbar-width:none]">
          {servers.filter(s => !mobile || s.id === active).map(s => (
            <div
              key={s.id}
              className={`no-drag group flex h-[30px] min-w-0 flex-none cursor-pointer items-center gap-2 rounded-lg border pr-1 pl-2.5 select-none ${mobile ? 'border-transparent pl-1 text-text' : s.id === active ? 'border-line-strong bg-panel-2 text-text' : 'border-transparent text-muted hover:text-text'}`}
              onClick={() => setActive(s.id)}
              title={`${s.user}@${s.host}`}
            >
              <span className={`h-2 w-2 flex-none rounded-full ${s.conn === 'ok' ? 'bg-ok shadow-[0_0_0_3px_rgba(63,207,142,.16)]' : s.conn === 'reconnecting' ? 'bg-warn' : 'bg-err'}`} />
              <span className="truncate text-[12.5px] font-semibold">{s.name}</span>
              {!mobile && <span className="font-mono text-[11px] text-faint">{s.host}</span>}
              {!mobile && (
                <button
                  className={`grid h-[22px] w-[22px] place-items-center rounded-[5px] text-muted group-hover:opacity-80 hover:bg-[color-mix(in_srgb,var(--text)_8%,transparent)] ${s.id === active ? 'opacity-80' : 'opacity-0'}`}
                  title={t('tabs.close')}
                  onClick={e => { e.stopPropagation(); close(s.id) }}
                ><Icon name="x" size={13} /></button>
              )}
            </div>
          ))}
        </div>
        {servers.length > 0 && !mobile && (
          <button className="grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" title={t('tabs.add')} onClick={() => setConnecting({})}>
            <Icon name="plus" />
          </button>
        )}
        {!mobile && <div className="flex-1 self-stretch" />}
        {cur && (
          <button
            className={`inline-flex h-7 flex-none items-center gap-1.5 rounded-lg border pr-2 pl-[9px] text-[12px] ${mobile ? 'ml-auto h-[30px]' : ''} ${sudo ? 'border-warn bg-[color-mix(in_srgb,var(--warn)_16%,transparent)] text-warn' : 'border-line-strong text-muted hover:text-text'}`}
            title={sudo ? t('sudo.on') : t('sudo.off')}
            data-sudo={sudo ? 'on' : 'off'}
            onClick={() => sudo ? turnOffSudo(cur.id) : requireSudo(cur.id)}
          >
            <Icon name="lock" size={14} />
            <span className="font-mono font-semibold">sudo</span>
            <span className={`relative h-[15px] w-[26px] rounded-full transition-colors ${sudo ? 'bg-warn' : 'bg-line-strong'}`}>
              <span className={`absolute top-0.5 left-0.5 h-[11px] w-[11px] rounded-full bg-white transition-transform ${sudo ? 'translate-x-[11px]' : ''}`} />
            </span>
          </button>
        )}
        {cur && !isRemote && !mobile && (
          <button className="inline-flex h-7 flex-none items-center gap-1.5 rounded-lg px-2.5 text-[12px] text-muted hover:bg-[color-mix(in_srgb,var(--text)_6%,transparent)] hover:text-text" onClick={openPair}>
            <Icon name="qr" size={15} />{t('pair.open')}
          </button>
        )}
        {mobile && cur && (
          <button className="grid h-[30px] w-[30px] flex-none place-items-center rounded-md text-muted hover:text-text" title={t('activity.settings')} onClick={() => setSettingsOpen(true)}>
            <Icon name="gear" />
          </button>
        )}
        {!isRemote && !mobile && <WindowButtons />}
      </header>

      <div className="relative min-h-0">
        {servers.length === 0 && <Welcome onConnect={preset => setConnecting({ preset })} onSettings={() => setSettingsOpen(true)} />}
        {servers.map(s => (
          <Workspace key={s.id} server={s} visible={s.id === active} onSettings={() => setSettingsOpen(true)} onReconnect={() => reconnect(s.id)} onClose={() => close(s.id)} />
        ))}
      </div>

      {mobile
        ? cur && <MobileNav tab={mtab} />
        : (
          <footer className="flex min-w-0 items-center gap-1 border-t border-line bg-chrome px-2 text-[11.5px] text-muted">
            {cur && (
              <span className="flex h-[22px] items-center gap-1.5 px-1.5 whitespace-nowrap" data-sb="conn">
                <span className={`h-2 w-2 flex-none rounded-full ${cur.conn === 'ok' ? 'bg-ok' : cur.conn === 'reconnecting' ? 'bg-warn' : 'bg-err'}`} />
                <span className="font-mono">{cur.user}@{cur.host}</span>
                {cur.conn !== 'ok' && <span className="text-err">· {t('conn.lostShort')}</span>}
              </span>
            )}
            {cur && !isRemote && <PhoneCount onClick={openPair} />}
            <div className="flex-1" />
            {cur && <StatusMetrics serverId={cur.id} />}
          </footer>
        )}

      {linkLost && (
        <div className="fixed inset-x-0 top-0 z-[60] bg-err px-3 py-2 text-center text-[12.5px] text-white" role="alert">{t('pair.linkLost')}</div>
      )}
      {connecting && <ConnectDialog preset={connecting.preset} onClose={() => setConnecting(null)} onConnected={added} />}
      {settingsOpen && <SettingsDialog onClose={() => setSettingsOpen(false)} />}
      {pairOpen && <PairDialog onClose={() => setPairOpen(false)} />}
      <SudoDialog who={id => { const s = servers.find(x => x.id === id); return s ? `${s.user}@${s.name}` : '' }} />
      <Toasts />
    </div>
  )
}

// 窓の枠を外しているので、最小化・最大化・閉じるを上の帯の右端に置く。閉じても通知領域に残る
function WindowButtons() {
  const { t } = useTranslation()
  const [max, setMax] = useState(false)
  useEffect(() => {
    const check = () => WindowIsMaximised().then(setMax)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])
  const btn = 'grid h-full w-[46px] place-items-center text-muted hover:bg-[color-mix(in_srgb,var(--text)_8%,transparent)] hover:text-text'
  return (
    <div className="-mr-2 flex flex-none self-stretch" data-winbtns>
      <button className={btn} title={t('desktop.minimize')} onClick={() => WindowMinimise()}><Icon name="min" size={14} /></button>
      <button className={btn} title={max ? t('desktop.restore') : t('desktop.maximize')} onClick={() => WindowToggleMaximise()}><Icon name={max ? 'restore' : 'max'} size={14} /></button>
      <button className={`${btn} hover:!bg-[#c42b1c] hover:!text-white`} title={t('desktop.close')} onClick={() => WindowHide()}><Icon name="x" size={14} /></button>
    </div>
  )
}

// スマホの下のタブ。ファイル・デスクトップ・ターミナルの区画を1つずつ出す
function MobileNav({ tab }: { tab: MTab }) {
  const { t } = useTranslation()
  const items: [MTab, IconName, string][] = [['files', 'files', t('mobile.files')], ['desktop', 'window', t('mobile.desktop')], ['terminal', 'terminal', t('mobile.terminal')]]
  return (
    <nav className="grid grid-cols-3 border-t border-line bg-chrome pb-[env(safe-area-inset-bottom)]">
      {items.map(([k, icon, label]) => (
        <button key={k} data-mtab={k} className={`grid content-center justify-items-center gap-[3px] text-[11px] ${tab === k ? 'text-accent' : 'text-faint'}`} onClick={() => setMTab(k)}>
          <Icon name={icon} size={21} />{label}
        </button>
      ))}
    </nav>
  )
}

// 起動した日時。M/D HH:MM
function bootedAt(uptime: number) {
  const d = new Date(Date.now() - uptime * 1000)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// 下の欄の稼働時間・CPU・メモリ・ネットワーク。押すとリソース監視の詳細を開く
function StatusMetrics({ serverId }: { serverId: string }) {
  const { t } = useTranslation()
  const sb = useSettings().sb
  const ms = useMetrics(serverId)
  const m = ms.at(-1)
  if (!m) return null
  const item = 'flex h-[22px] items-center gap-1.5 rounded px-1.5 whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--text)_6%,transparent)] hover:text-text'
  const gauge = (key: 'cpu' | 'mem', label: string, values: number[], text: string, cls = '') => (
    <button className={`${item} ${cls}`} data-sb={key} title={t('mon.detailOf', { name: label })} onClick={() => openTool(serverId, 'monitor', key)}>
      {label}<Spark values={values} max={100} className="sb" /><span className="num">{text}</span>
    </button>
  )
  return <>
    {sb.uptime && <span className={item.replace(/hover:\S+/g, '')} data-sb="uptime" title={t('mon.uptimeHint', { when: bootedAt(m.uptime) })}><Icon name="clock" size={13} />{fmtUptime(m.uptime, t)}</span>}
    {sb.cpu && gauge('cpu', 'CPU', ms.map(x => x.cpu), `${m.cpu.toFixed(0)}%`)}
    {/* メモリは使っている量と全体の量を出す。グラフは割合 */}
    {sb.mem && gauge('mem', t('mon.mem'), ms.map(x => pct(x.memUsed, x.memTotal)), `${fmtGB(m.memUsed)} / ${fmtGB(m.memTotal)} GB`, 'sb-mem')}
    {sb.net && (
      <button className={item} data-sb="net" title={t('mon.detailOf', { name: t('mon.net') })} onClick={() => openTool(serverId, 'monitor', 'net')}>
        <span className="text-ok" title={t('mon.down')}><Icon name="down" size={13} /></span><span className="num">{fmtRate(m.rx)}</span>
        <span className="text-accent" title={t('mon.up')}><Icon name="up" size={13} /></span><span className="num">{fmtRate(m.tx)}</span>
      </button>
    )}
  </>
}

// 下の欄の「スマホ N台」。押すとスマホの登録の窓を開く
function PhoneCount({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  const { devices } = usePhoneDevices()
  return (
    <button className="flex h-[22px] items-center gap-1.5 rounded px-1.5 whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--text)_6%,transparent)] hover:text-text" title={t('pair.open')} onClick={onClick}>
      <Icon name="phone" size={13} />{t('pair.count', { n: devices.length })}
    </button>
  )
}

function Workspace({ server: s, visible, onSettings, onReconnect, onClose }: {
  server: Server
  visible: boolean
  onSettings: () => void
  onReconnect: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const st = useSettings()
  const [view, setViewState] = useState<'files' | 'sessions'>('files')
  // 左端で、いま出している一覧のボタンをもう一度押すと、サイドバーを畳む
  const [collapsed, setCollapsed] = useState(false)
  const setView = (v: 'files' | 'sessions') => {
    setCollapsed(false)
    setViewState(v)
  }
  const pick = (v: 'files' | 'sessions') => view === v ? setCollapsed(c => !c) : setView(v)
  // ほかの窓の「ツリーで表示」で、ファイルの一覧に切り替える
  useEffect(() => {
    const onReveal = (e: Event) => {
      const d = (e as CustomEvent<{ serverId: string; show?: boolean }>).detail
      if (d.serverId !== s.id || !d.show) return
      setView('files')
      setMTab('files')
    }
    window.addEventListener('drag:tree-reveal', onReveal)
    return () => window.removeEventListener('drag:tree-reveal', onReveal)
  }, [s.id])
  // PC のファイルをドラッグしてきたら、落とせるようにファイルの一覧に切り替える
  useEffect(() => {
    if (!visible) return
    const onEnter = (e: DragEvent) => { if (e.dataTransfer?.types.includes('Files')) setView('files') }
    window.addEventListener('dragenter', onEnter)
    return () => window.removeEventListener('dragenter', onEnter)
  }, [visible])
  const [termFg, setTermFg] = useState('')
  // スマホの幅でも同じ部品を同じ並びで置き、区画を隠すだけにする。
  // 向きを変えて幅が変わっても、ターミナルのシェルやツリーの開き具合が作り直されないように
  const mobile = useMobile()
  const mtab = useMTab()
  const pane = (k: MTab) => mobile ? (mtab === k ? 'flex min-h-0 min-w-0 flex-1 flex-col' : 'hidden') : 'contents'
  return (
    <div
      className={`absolute inset-0 ${mobile ? 'flex flex-col' : 'grid'} ${visible ? '' : 'invisible'}`}
      style={mobile ? undefined : { gridTemplateColumns: collapsed ? '48px 0 0 minmax(0,1fr)' : `48px ${Math.max(SIDEBAR_MIN, st.sidebarW)}px 4px minmax(0,1fr)` }}
      data-server={s.id}
    >
      {!mobile && (
        <nav className="flex flex-col items-center gap-1 border-r border-line bg-chrome pt-1.5 pb-1.5">
          <ActButton icon="files" title={t('activity.files')} active={view === 'files' && !collapsed} onClick={() => pick('files')} />
          <ActButton icon="layers" title={t('activity.sessions')} active={view === 'sessions' && !collapsed} onClick={() => pick('sessions')} />
          <div className="flex-1" />
          <ActButton icon="gear" title={t('activity.settings')} onClick={onSettings} />
        </nav>
      )}
      <aside className={mobile ? (mtab === 'files' ? 'flex min-h-0 flex-1 flex-col bg-panel' : 'hidden') : `flex min-w-0 flex-col overflow-hidden border-r border-line bg-panel ${collapsed ? 'invisible' : ''}`}>
        {mobile
          ? (
            <div className="mx-3 mt-2.5 mb-0.5 flex gap-1 rounded-[10px] border border-line bg-field p-[3px]">
              {(['files', 'sessions'] as const).map(v => (
                <button key={v} className={`h-8 flex-1 rounded-lg ${view === v ? 'bg-raised text-text' : 'text-muted'}`} onClick={() => setView(v)}>{t(`activity.${v}`)}</button>
              ))}
            </div>
          )
          : view === 'sessions' && <div className="flex h-9 flex-none items-center pr-1.5 pl-3.5 text-[11px] font-semibold tracking-[.08em] text-muted">{t('sessions.title')}</div>}
        {/* ツリーの開き具合を残すため、隠すだけにする */}
        <div className={`relative flex min-h-0 flex-1 flex-col ${view === 'files' ? '' : 'hidden'}`}>
          <FileTree serverId={s.id} root={s.home} generation={s.gen} />
        </div>
        {view === 'sessions' && (
          <SessionsView serverId={s.id} visible={visible} terminalKey={sessionKeyOf(termFg)} onOpen={x => openSessionWindow(s.id, x)} />
        )}
      </aside>
      {!mobile && (collapsed ? <div /> : <Resizer dir="v" value={Math.max(SIDEBAR_MIN, st.sidebarW)} min={SIDEBAR_MIN} max={520} onChange={sidebarW => setSettings({ sidebarW })} />)}
      <main
        className={mobile ? (mtab === 'files' ? 'hidden' : 'flex min-h-0 flex-1 flex-col') : 'grid min-h-0 min-w-0'}
        style={mobile ? undefined : { gridTemplateRows: `minmax(0,1fr) 4px ${st.terminalH}px` }}
      >
        <div className={pane('desktop')}><Desktop serverId={s.id} home={s.home} generation={s.gen} /></div>
        {!mobile && <Resizer dir="h" value={st.terminalH} min={120} max={Math.max(160, window.innerHeight - 260)} onChange={terminalH => setSettings({ terminalH })} invert />}
        <div className={pane('terminal')}>
          <TerminalPanel serverId={s.id} home={s.home} generation={s.gen} visible={visible && (!mobile || mtab === 'terminal')} fontSize={st.termSize} onForegroundChange={setTermFg} />
        </div>
      </main>
      {s.conn !== 'ok' && <ConnBanner server={s} onReconnect={onReconnect} onClose={onClose} />}
    </div>
  )
}

// 接続が切れたときの表示。後ろの作業区画は暗くして触れないようにする
function ConnBanner({ server: s, onReconnect, onClose }: { server: Server; onReconnect: () => void; onClose: () => void }) {
  const { t } = useTranslation()
  const small = 'h-7 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover'
  return (
    <div className="absolute inset-0 z-40 bg-[color-mix(in_srgb,var(--bg)_50%,transparent)] backdrop-grayscale-[.6]">
      <div className="absolute top-3.5 left-1/2 flex w-[min(560px,calc(100%-24px))] -translate-x-1/2 items-center gap-3 rounded-xl border border-[color-mix(in_srgb,var(--err)_55%,transparent)] bg-raised px-3.5 py-3 shadow-win" role="alert">
        {s.conn === 'reconnecting'
          ? <span className="h-4 w-4 flex-none animate-spin rounded-full border-2 border-warn border-t-transparent" />
          : <Icon name="link" className="text-err" />}
        <div className="min-w-0 flex-1">
          <div className="font-semibold">{t('conn.lost', { name: s.name })}</div>
          <div className="text-[12px] text-muted">
            {s.error || (s.conn === 'reconnecting' ? t('conn.reconnecting') : t('conn.retryIn', { wait: s.wait, n: s.retry }))}
          </div>
          <div className="text-[12px] text-muted">{t('conn.shellEnded')}</div>
        </div>
        <button className={small} disabled={s.conn === 'reconnecting'} onClick={onReconnect}>{t('conn.now')}</button>
        <button className={small} onClick={onClose}>{t('conn.disconnect')}</button>
      </div>
    </div>
  )
}

function ActButton({ icon, title, active = false, onClick }: { icon: IconName; title: string; active?: boolean; onClick?: () => void }) {
  return (
    <button
      className={`relative grid h-10 w-10 place-items-center rounded-lg ${active ? 'text-text before:absolute before:top-[9px] before:bottom-[9px] before:-left-1 before:w-0.5 before:rounded-sm before:bg-accent' : 'text-faint hover:text-text'}`}
      title={title}
      onClick={onClick}
    >
      <Icon name={icon} size={21} />
    </button>
  )
}

function Welcome({ onConnect, onSettings }: { onConnect: (preset?: store.Server) => void; onSettings: () => void }) {
  const { t } = useTranslation()
  const [saved, setSaved] = useState<store.Server[]>([])
  useEffect(() => { SavedServers().then(list => setSaved(list ?? [])) }, [])
  return (
    <section className="desk-bg absolute inset-0 grid place-items-center p-6">
      <button className="absolute bottom-3 left-2 grid h-10 w-10 place-items-center rounded-lg text-faint hover:text-text" title={t('activity.settings')} onClick={onSettings}>
        <Icon name="gear" size={21} />
      </button>
      <div className="grid w-[min(560px,100%)] justify-items-center gap-2.5 text-center">
        {/* ロゴの字は紺色なので、暗いテーマでは字を明るくしたものを出す */}
        <img src={logoDark} alt={t('brandSub')} draggable={false} className="logo-dark w-[min(360px,100%)]" />
        <img src={logoLight} alt={t('brandSub')} draggable={false} className="logo-light w-[min(360px,100%)]" />
        <button className="btn-primary mt-[18px] mb-2 flex h-10 items-center gap-1.5 rounded-lg px-[22px] text-[14px]" onClick={() => onConnect()}>
          <Icon name="plus" />{t('connect.openDialog')}
        </button>
        {saved.length > 0 && (
          <div className="mt-2.5 w-full text-left">
            <span className="mb-1.5 block text-[12px] text-muted">{t('connect.saved')}</span>
            <div className="grid grid-cols-3 gap-2">
              {saved.map(s => (
                <button key={s.id} className="flex min-w-0 items-center gap-2.5 rounded-[10px] border border-line-strong bg-panel-2 p-2.5 text-left hover:border-faint" onClick={() => onConnect(s)}>
                  <Icon name="server" className="text-muted" />
                  <div className="min-w-0">
                    <div className="font-semibold">{s.name}</div>
                    <div className="truncate font-mono text-[11px] text-muted">{s.user}@{s.host}</div>
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </section>
  )
}
