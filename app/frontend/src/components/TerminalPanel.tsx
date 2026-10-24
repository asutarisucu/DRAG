import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloseShell, OpenShell, ShellCwd, ShellInput, ShellResize } from '../../wailsjs/go/main/App'
import { attachShell, foregroundOf, onForeground } from '../shellBus'
import { useSettings } from '../settings'
import { registerTerminal } from '../terminals'
import { toast } from '../toasts'
import { openWin } from '../windows'
import { Icon } from './Icon'
import { useXterm } from './useXterm'

// コマンドの行を、先頭の sudo を飛ばした語の並びにする
function words(cmdline: string) {
  const w = cmdline.trim().split(/\s+/)
  if (w[0]?.split('/').pop() === 'sudo') {
    w.shift()
    while (w[0]?.startsWith('-')) w.shift()
  }
  if (w[0]) w[0] = w[0].split('/').pop()!
  return w
}

// 自動でウィンドウに移すコマンドか。「docker exec」のように2語のものは2語とも見る
export function matchesAuto(cmdline: string, list: string[]) {
  const w = words(cmdline)
  return list.some(e => e.split(' ').every((x, i) => w[i] === x))
}

const isDetachable = (cmdline: string) => ['screen', 'tmux'].includes(words(cmdline)[0])

// 右下のターミナル。サーバーごとに1つシェルを開く。
// generation は再接続のたびに増える。増えたら同じ画面のまま新しいシェルを開き直す。
// 前面で動いているコマンドは見出しに出し、ドラッグかクリックでデスクトップのウィンドウに移す
export function TerminalPanel({ serverId, generation, visible, fontSize }: {
  serverId: string
  generation: number
  visible: boolean
  fontSize: number
}) {
  const { t } = useTranslation()
  const settings = useSettings()
  const host = useRef<HTMLDivElement>(null)
  const { termRef, fitRef } = useXterm(host, fontSize)
  const shellRef = useRef('')
  const detachRef = useRef(() => {})
  const [fg, setFg] = useState('')
  // 言語や設定を変えてもシェルを開き直さないよう、effect の中では ref から読む
  const tRef = useRef(t)
  tRef.current = t
  const settingsRef = useRef(settings)
  settingsRef.current = settings

  const dim = (text: string) => termRef.current?.write(`\r\n\x1b[2m${text}\x1b[0m\r\n`)

  const open = async (cwd: string) => {
    const term = termRef.current
    if (!term) return
    try {
      const id = await OpenShell(serverId, term.cols, term.rows, cwd)
      if (termRef.current !== term) {
        CloseShell(id)
        return
      }
      shellRef.current = id
      detachRef.current = attachShell(id, {
        out: data => term.write(data),
        exit: () => {
          if (shellRef.current === id) shellRef.current = ''
          dim(tRef.current('terminal.ended'))
        },
      })
    } catch (err) {
      term.write(`\x1b[31m${String(err)}\x1b[0m\r\n`)
    }
  }

  useEffect(() => {
    const term = termRef.current!
    if (generation > 0) dim(tRef.current('terminal.reopened'))
    open('')
    const onData = term.onData(d => { if (shellRef.current) ShellInput(shellRef.current, d) })
    const onResize = term.onResize(({ cols, rows }) => { if (shellRef.current) ShellResize(shellRef.current, cols, rows) })
    return () => {
      onData.dispose()
      onResize.dispose()
      detachRef.current()
      if (shellRef.current) CloseShell(shellRef.current)
      shellRef.current = ''
      setFg('')
    }
  }, [serverId, generation, termRef])

  // 前面のコマンドを移す。今のチャネルはそのままウィンドウに渡し、ここには同じフォルダで新しいシェルを開く
  const popOut = async (pos?: { x: number; y: number }) => {
    const id = shellRef.current
    const cmd = foregroundOf(id)
    if (!id || !cmd) return
    detachRef.current()
    shellRef.current = ''
    setFg('')
    dim(tRef.current('desktop.moved', { cmd }))
    const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
    openWin({
      serverId, kind: 'session', title: cmd, icon: 'terminal', shellId: id,
      detachable: isDetachable(cmd), replayFromIdle: true, rect: pos,
    }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
    open(await ShellCwd(id).catch(() => ''))
  }
  const popOutRef = useRef(popOut)
  popOutRef.current = popOut

  useEffect(() => onForeground((id, cmdline) => {
    if (id !== shellRef.current) return
    setFg(cmdline)
    const s = settingsRef.current
    if (cmdline && s.autoOn && matchesAuto(cmdline, s.autoCmds)) popOutRef.current()
  }), [])

  // ファイルツリーから cd やパスを送る窓口。前面で別のコマンドが動いているときは送らない
  useEffect(() => {
    const send = (text: string) => {
      const id = shellRef.current
      if (!id) return
      if (foregroundOf(id)) {
        toast(tRef.current('toast.busy'), 'warn')
        return
      }
      ShellInput(id, text)
      termRef.current?.focus()
    }
    return registerTerminal(serverId, {
      cd: path => send(`cd ${path}\r`),
      insert: text => send(text),
      popOut: pos => popOutRef.current(pos),
      cwd: () => shellRef.current ? ShellCwd(shellRef.current) : Promise.resolve(''),
    })
  }, [serverId, termRef])

  useEffect(() => {
    if (visible) {
      fitRef.current?.fit()
      termRef.current?.focus()
    }
  }, [visible, fitRef, termRef])

  return (
    <section className="flex min-h-0 flex-col border-t border-line bg-term">
      <div className="flex h-[34px] flex-none items-center gap-2.5 border-b border-line bg-panel pr-2 pl-3.5">
        <span className="text-[11px] font-semibold tracking-[.08em] text-muted">{t('terminal.title')}</span>
        {fg && (
          <div
            draggable
            role="button"
            className="inline-flex h-6 min-w-0 cursor-grab items-center gap-2 rounded-full border border-[color-mix(in_srgb,var(--accent)_45%,transparent)] bg-accent-soft pr-3 pl-1.5 select-none active:cursor-grabbing"
            onDragStart={e => {
              e.dataTransfer.setData('application/x-drag-fg', serverId)
              e.dataTransfer.effectAllowed = 'move'
            }}
            onClick={() => popOut()}
            title={fg}
          >
            <Icon name="grip" className="text-accent" />
            <span className="max-w-[260px] truncate font-mono text-[12px]">{fg}</span>
            <span className="text-[11px] whitespace-nowrap text-muted">{t('desktop.chipHint')}</span>
          </div>
        )}
      </div>
      <div ref={host} className="min-h-0 flex-1" />
    </section>
  )
}
