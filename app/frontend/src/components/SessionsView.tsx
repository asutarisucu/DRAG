import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { OtherShells, Sessions } from '../../wailsjs/go/main/App'
import type { main, remote } from '../../wailsjs/go/models'
import { setMTab, useMobile } from '../mobile'
import { sameSession } from '../sessions'
import { focusWin, openWin, useWins } from '../windows'
import { Icon } from './Icon'

const pad2 = (n: number) => String(n).padStart(2, '0')
// 最後に使った時刻。今日なら時刻だけ、今年なら月日と時刻
function when(sec: number) {
  const d = new Date(sec * 1000), now = new Date()
  const time = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  if (d.toDateString() === now.toDateString()) return time
  return d.getFullYear() === now.getFullYear() ? `${d.getMonth() + 1}/${d.getDate()} ${time}` : `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`
}

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
  const [others, setOthers] = useState<main.ShellInfo[]>([])
  const wins = useWins(serverId)
  const mobile = useMobile()
  const pad = mobile ? 'py-2.5 px-4' : 'py-1.5 px-3.5'

  useEffect(() => {
    if (!visible) return
    const load = () => {
      Sessions(serverId).then(l => setList(l ?? []), () => setList([]))
      OtherShells(serverId).then(l => setOthers(l ?? []), () => setOthers([]))
    }
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
        key={s.kind + s.user + s.name}
        draggable={!mobile}
        data-session={`${s.kind}:${s.user ? s.user + '/' : ''}${s.name}`}
        className={`flex cursor-pointer items-center gap-2.5 select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${pad}`}
        onClick={() => onOpen(s)}
        onDragStart={e => e.dataTransfer.setData('application/x-drag-session', JSON.stringify({ kind: s.kind, name: s.name, user: s.user }))}
      >
        <Icon name="layers" className="text-accent-2" />
        <div className="min-w-0">
          <div className="truncate font-mono text-[12.5px]">{s.name}</div>
          <div className="text-[11.5px] text-muted">
            {[s.user ? t('sessions.ownedBy', { user: s.user }) : '', s.kind === 'tmux' ? t('sessions.windowsCount', { n: s.desc }) : '', s.lastUsed ? t('sessions.lastUsed', { when: when(s.lastUsed) }) : s.kind === 'screen' ? s.desc : ''].filter(Boolean).join(' · ')}
          </div>
        </div>
        {where && <span className="ml-auto flex-none rounded-full bg-accent-soft px-2 py-px text-[10.5px] text-accent">{where}</span>}
      </div>
    )
  }

  const own = wins.filter(w => !w.detachable && !w.shared)
  // ほかの端末で開いているシェル。開くとその端末の大きさのままウィンドウに出す
  const openOther = (sh: main.ShellInfo) => {
    setMTab('desktop')
    const open = wins.find(w => w.shellId === sh.id)
    if (open) {
      focusWin(open.id)
      return
    }
    const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
    openWin({
      serverId, kind: 'terminal', title: sh.fg || t('sessions.shellOf', { owner: sh.owner }), icon: 'terminal', shellId: sh.id,
      detachable: false, replayFromIdle: false, shared: { cols: sh.cols, rows: sh.rows }, rect: { w: 640, h: 360 },
    }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto pb-5">
      {list && [
        // ほかのユーザーの screen は、自分のものの後ろに並べる (見えるのは root か sudo モードのときだけ)
        ...group('screen', [...list.filter(s => s.kind === 'screen' && !s.user), ...list.filter(s => s.kind === 'screen' && s.user)].map(item)),
        ...group('tmux', list.filter(s => s.kind === 'tmux').map(item)),
        ...group(t('sessions.windows'), [...own.map(w => (
          <div key={w.id} className={`flex cursor-pointer items-center gap-2.5 select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${pad}`} onClick={() => { focusWin(w.id); setMTab('desktop') }}>
            <Icon name={w.icon} className="text-accent-2" />
            <div className="min-w-0">
              <div className="truncate font-mono text-[12.5px]">{w.title}</div>
              <div className="text-[11.5px] text-muted">{w.kind === 'terminal' ? t('sessions.shell') : w.title}</div>
            </div>
          </div>
        )), ...others.map(sh => (
          <div key={sh.id} data-other-shell={sh.id} className={`flex cursor-pointer items-center gap-2.5 select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${pad}`} onClick={() => openOther(sh)}>
            <Icon name="terminal" className="text-accent-2" />
            <div className="min-w-0">
              <div className="truncate font-mono text-[12.5px]">{sh.fg || t('sessions.shell')}</div>
              <div className="text-[11.5px] text-muted">{t('sessions.openOn', { owner: sh.owner })}</div>
            </div>
            {wins.some(w => w.shellId === sh.id) && <span className="ml-auto flex-none rounded-full bg-accent-soft px-2 py-px text-[10.5px] text-accent">{t('sessions.inWindow')}</span>}
          </div>
        ))]),
      ]}
    </div>
  )
}
