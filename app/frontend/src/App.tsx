import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Disconnect } from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { ConnectDialog, type Connected } from './components/ConnectDialog'
import { FileTree } from './components/FileTree'
import { Icon, type IconName } from './components/Icon'
import { TerminalPanel } from './components/TerminalPanel'

type Server = Connected & { lost: boolean }

// 全体の枠。上がサーバーのタブ、左がファイルツリー、右上がデスクトップ、右下がターミナル
export default function App() {
  const { t } = useTranslation()
  const [servers, setServers] = useState<Server[]>([])
  const [active, setActive] = useState('')
  const [connecting, setConnecting] = useState(false)

  useEffect(() => EventsOn('server:closed', (id: string) => {
    setServers(list => list.map(s => s.id === id ? { ...s, lost: true } : s))
  }), [])

  const added = (c: Connected) => {
    setServers(list => [...list, { ...c, lost: false }])
    setActive(c.id)
    setConnecting(false)
  }
  const close = (id: string) => {
    Disconnect(id)
    setServers(list => {
      const rest = list.filter(s => s.id !== id)
      if (active === id) setActive(rest.at(-1)?.id ?? '')
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
              <span className={`h-2 w-2 flex-none rounded-full ${s.lost ? 'bg-err' : 'bg-ok shadow-[0_0_0_3px_rgba(63,207,142,.16)]'}`} />
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
          <button className="mr-auto grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" title={t('tabs.add')} onClick={() => setConnecting(true)}>
            <Icon name="plus" />
          </button>
        )}
      </header>

      <div className="relative min-h-0">
        {servers.length === 0 && <Welcome onConnect={() => setConnecting(true)} />}
        {servers.map(s => (
          <Workspace key={s.id} server={s} visible={s.id === active} />
        ))}
      </div>

      <footer className="flex min-w-0 items-center gap-1 border-t border-line bg-chrome px-2 text-[11.5px] text-muted">
        {cur && (
          <span className="flex h-[22px] items-center gap-1.5 px-1.5 whitespace-nowrap">
            <Icon name="server" size={13} />
            <span className="font-mono">{cur.user}@{cur.host}</span>
          </span>
        )}
        {cur?.lost && <span className="px-1.5 text-err">{t('status.lost', { name: cur.name })}</span>}
      </footer>

      {connecting && <ConnectDialog onClose={() => setConnecting(false)} onConnected={added} />}
    </div>
  )
}

function Workspace({ server, visible }: { server: Server; visible: boolean }) {
  const { t } = useTranslation()
  return (
    <div className={`absolute inset-0 grid grid-cols-[48px_260px_minmax(0,1fr)] ${visible ? '' : 'invisible'}`}>
      <nav className="flex flex-col items-center gap-1 border-r border-line bg-chrome pt-1.5">
        <ActButton icon="files" title={t('activity.files')} active />
        <ActButton icon="layers" title={t('activity.sessions')} />
        <div className="flex-1" />
        <ActButton icon="gear" title={t('activity.settings')} />
      </nav>
      <aside className="flex min-w-0 flex-col overflow-hidden border-r border-line bg-panel">
        <div className="flex h-9 flex-none items-center pr-1.5 pl-3.5 text-[11px] font-semibold tracking-[.08em] text-muted">{t('files.title')}</div>
        <FileTree serverId={server.id} root={server.home} />
      </aside>
      <main className="grid min-h-0 min-w-0 grid-rows-[minmax(0,1fr)_230px]">
        <section className="desk-bg relative grid min-h-0 place-items-center text-faint select-none">{t('desktop.empty')}</section>
        <TerminalPanel serverId={server.id} visible={visible} />
      </main>
    </div>
  )
}

function ActButton({ icon, title, active = false }: { icon: IconName; title: string; active?: boolean }) {
  return (
    <button
      className={`relative grid h-10 w-10 place-items-center rounded-lg ${active ? 'text-text before:absolute before:top-[9px] before:bottom-[9px] before:-left-1 before:w-0.5 before:rounded-sm before:bg-accent' : 'text-faint hover:text-text'}`}
      title={title}
    >
      <Icon name={icon} size={21} />
    </button>
  )
}

function Welcome({ onConnect }: { onConnect: () => void }) {
  const { t } = useTranslation()
  return (
    <section className="desk-bg absolute inset-0 grid place-items-center p-6">
      <div className="grid w-[min(560px,100%)] justify-items-center gap-2.5 text-center">
        <div className="brand pl-[.2em] text-[40px] tracking-[.2em]">DRAG</div>
        <div className="text-muted">{t('brandSub')}</div>
        <button className="btn-primary mt-[18px] mb-2 flex h-10 items-center gap-1.5 rounded-lg px-[22px] text-[14px]" onClick={onConnect}>
          <Icon name="plus" />{t('connect.open')}
        </button>
      </div>
    </section>
  )
}
