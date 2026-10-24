import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Sessions } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { setMTab, useMobile } from '../mobile'
import { sameSession } from '../sessions'
import { focusWin, useWins } from '../windows'
import { Icon } from './Icon'

// 左の欄の「セッション」。screen と tmux のセッションと、DRAG のウィンドウを並べる。
// 見えている間は数秒おきに読み直す
export function SessionsView({ serverId, visible, terminalKey, onOpen }: {
  serverId: string
  visible: boolean
  terminalKey: string // 右下のターミナルで開いているセッション
  onOpen: (s: remote.Session) => void
}) {
  const { t } = useTranslation()
  const [list, setList] = useState<remote.Session[] | null>(null)
  const wins = useWins(serverId)
  const mobile = useMobile()
  const pad = mobile ? 'py-2.5 px-4' : 'py-1.5 px-3.5'

  useEffect(() => {
    if (!visible) return
    const load = () => Sessions(serverId).then(l => setList(l ?? []), () => setList([]))
    load()
    const iv = setInterval(load, 4000)
    return () => clearInterval(iv)
  }, [serverId, visible])

  const group = (label: string, items: React.ReactNode[]) => [
    <div key={label} className="px-3.5 pt-3 pb-1 text-[11px] font-semibold tracking-[.06em] text-faint">{label}</div>,
    ...(items.length ? items : [<div key={label + ':none'} className="px-3.5 py-1 text-[12px] text-faint">{t('sessions.none')}</div>]),
  ]
  const item = (s: remote.Session) => {
    const where = wins.some(w => sameSession(s, w.sessionKey)) ? t('sessions.inWindow') : sameSession(s, terminalKey) ? t('sessions.inTerminal') : ''
    return (
      <div
        key={s.kind + s.name}
        draggable={!mobile}
        data-session={`${s.kind}:${s.name}`}
        className={`flex cursor-pointer items-center gap-2.5 select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${pad}`}
        onClick={() => onOpen(s)}
        onDragStart={e => e.dataTransfer.setData('application/x-drag-session', JSON.stringify({ kind: s.kind, name: s.name }))}
      >
        <Icon name="layers" className="text-accent-2" />
        <div className="min-w-0">
          <div className="truncate font-mono text-[12.5px]">{s.name}</div>
          <div className="text-[11.5px] text-muted">{s.kind === 'tmux' ? t('sessions.windowsCount', { n: s.desc }) : s.desc}</div>
        </div>
        {where && <span className="ml-auto flex-none rounded-full bg-accent-soft px-2 py-px text-[10.5px] text-accent">{where}</span>}
      </div>
    )
  }

  const own = wins.filter(w => !w.detachable)
  return (
    <div className="min-h-0 flex-1 overflow-auto pb-5">
      {list && [
        ...group('screen', list.filter(s => s.kind === 'screen').map(item)),
        ...group('tmux', list.filter(s => s.kind === 'tmux').map(item)),
        ...group(t('sessions.windows'), own.map(w => (
          <div key={w.id} className={`flex cursor-pointer items-center gap-2.5 select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${pad}`} onClick={() => { focusWin(w.id); setMTab('desktop') }}>
            <Icon name={w.icon} className="text-accent-2" />
            <div className="min-w-0">
              <div className="truncate font-mono text-[12.5px]">{w.title}</div>
              <div className="text-[11.5px] text-muted">{w.kind === 'terminal' ? t('sessions.shell') : w.title}</div>
            </div>
          </div>
        ))),
      ]}
    </div>
  )
}
