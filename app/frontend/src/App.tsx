import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Disconnect, Reconnect, SavedServers } from '../wailsjs/go/main/App'
import type { store } from '../wailsjs/go/models'
import { EventsOn, OnFileDrop, OnFileDropOff } from '../wailsjs/runtime/runtime'
import { ConnectDialog, type Connected } from './components/ConnectDialog'
import { FileTree } from './components/FileTree'
import { Icon, type IconName } from './components/Icon'
import { Resizer } from './components/Resizer'
import { SettingsDialog } from './components/SettingsDialog'
import { TerminalPanel } from './components/TerminalPanel'
import { Toasts } from './components/Toasts'
import { setSettings, useSettings } from './settings'

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
  const update = (id: string, f: (s: Server) => Partial<Server>) => setServers(list => list.map(s => s.id === id ? { ...s, ...f(s) } : s))

  useEffect(() => EventsOn('server:closed', (id: string) => {
    update(id, s => s.conn === 'ok' ? { conn: 'lost', retry: 1, wait: 3, error: '' } : {})
  }), [])

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
      else update(id, s => ({ conn: 'ok', retry: 0, gen: s.gen + 1, home: r.home }))
    })
  }, [])

  useEffect(() => {
    for (const s of servers) if (s.conn === 'lost' && !s.error && s.wait <= 0) reconnect(s.id)
  }, [servers, reconnect])

  const added = (c: Connected) => {
    setServers(list => [...list, { ...c, conn: 'ok', retry: 0, wait: 0, gen: 0, error: '' }])
    setActive(c.id)
    setConnecting(null)
  }
  const close = (id: string) => {
    Disconnect(id)
    setServers(list => {
      const rest = list.filter(s => s.id !== id)
      setActive(a => a === id ? rest.at(-1)?.id ?? '' : a)
      return rest
    })
  }
  const cur = servers.find(s => s.id === active)

  return (
    <div className="grid h-dvh grid-rows-[42px_minmax(0,1fr)_24px]">
      <header className="flex min-w-0 items-center gap-2 border-b border-line bg-chrome px-2">
        <span className="brand pr-2 pl-1.5 text-[13px]">DRAG</span>
        <div className="flex min-w-0 gap-1 overflow-x-auto [scrollbar-width:none]">
          {servers.map(s => (
            <div
              key={s.id}
              className={`group flex h-[30px] flex-none cursor-pointer items-center gap-2 rounded-lg border pr-1 pl-2.5 select-none ${s.id === active ? 'border-line-strong bg-panel-2 text-text' : 'border-transparent text-muted hover:text-text'}`}
              onClick={() => setActive(s.id)}
            >
              <span className={`h-2 w-2 flex-none rounded-full ${s.conn === 'ok' ? 'bg-ok shadow-[0_0_0_3px_rgba(63,207,142,.16)]' : s.conn === 'reconnecting' ? 'bg-warn' : 'bg-err'}`} />
              <span className="text-[12.5px] font-semibold">{s.name}</span>
              <span className="font-mono text-[11px] text-faint">{s.host}</span>
              <button
                className="grid h-[22px] w-[22px] place-items-center rounded-[5px] text-muted opacity-0 group-hover:opacity-80 hover:bg-[color-mix(in_srgb,var(--text)_8%,transparent)]"
                title={t('tabs.close')}
                onClick={e => { e.stopPropagation(); close(s.id) }}
              ><Icon name="x" size={13} /></button>
            </div>
          ))}
        </div>
        {servers.length > 0 && (
          <button className="mr-auto grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" title={t('tabs.add')} onClick={() => setConnecting({})}>
            <Icon name="plus" />
          </button>
        )}
      </header>

      <div className="relative min-h-0">
        {servers.length === 0 && <Welcome onConnect={preset => setConnecting({ preset })} onSettings={() => setSettingsOpen(true)} />}
        {servers.map(s => (
          <Workspace key={s.id} server={s} visible={s.id === active} onSettings={() => setSettingsOpen(true)} onReconnect={() => reconnect(s.id)} onClose={() => close(s.id)} />
        ))}
      </div>

      <footer className="flex min-w-0 items-center gap-1 border-t border-line bg-chrome px-2 text-[11.5px] text-muted">
        {cur && (
          <span className="flex h-[22px] items-center gap-1.5 px-1.5 whitespace-nowrap">
            <Icon name="server" size={13} />
            <span className="font-mono">{cur.user}@{cur.host}</span>
          </span>
        )}
      </footer>

      {connecting && <ConnectDialog preset={connecting.preset} onClose={() => setConnecting(null)} onConnected={added} />}
      {settingsOpen && <SettingsDialog onClose={() => setSettingsOpen(false)} />}
      <Toasts />
    </div>
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
  return (
    <div className={`absolute inset-0 grid ${visible ? '' : 'invisible'}`} style={{ gridTemplateColumns: `48px ${st.sidebarW}px 4px minmax(0,1fr)` }} data-server={s.id}>
      <nav className="flex flex-col items-center gap-1 border-r border-line bg-chrome pt-1.5 pb-1.5">
        <ActButton icon="files" title={t('activity.files')} active />
        <ActButton icon="layers" title={t('activity.sessions')} />
        <div className="flex-1" />
        <ActButton icon="gear" title={t('activity.settings')} onClick={onSettings} />
      </nav>
      <aside className="flex min-w-0 flex-col overflow-hidden border-r border-line bg-panel">
        <div className="flex h-9 flex-none items-center pr-1.5 pl-3.5 text-[11px] font-semibold tracking-[.08em] text-muted">{t('files.title')}</div>
        <FileTree serverId={s.id} root={s.home} generation={s.gen} />
      </aside>
      <Resizer dir="v" value={st.sidebarW} min={180} max={520} onChange={sidebarW => setSettings({ sidebarW })} />
      <main className="grid min-h-0 min-w-0" style={{ gridTemplateRows: `minmax(0,1fr) 4px ${st.terminalH}px` }}>
        <section className="desk-bg relative grid min-h-0 place-items-center text-faint select-none">{t('desktop.empty')}</section>
        <Resizer dir="h" value={st.terminalH} min={120} max={Math.max(160, window.innerHeight - 260)} onChange={terminalH => setSettings({ terminalH })} invert />
        <TerminalPanel serverId={s.id} generation={s.gen} visible={visible} fontSize={st.termSize} />
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
        <div className="brand pl-[.2em] text-[40px] tracking-[.2em]">DRAG</div>
        <div className="text-muted">{t('brandSub')}</div>
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
