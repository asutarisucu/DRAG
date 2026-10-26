import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloseShell, OpenShell, ShellCwd, ShellReplay, ShellResize } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { attachCommand, sessionKeyOf } from '../sessions'
import { attachShell, decode, foregroundOf, onForeground } from '../shellBus'
import { getMTab, isMobile, setMTab, useMobile } from '../mobile'
import { sendInput, typeLine } from '../shellInput'
import { useSettings } from '../settings'
import { pathArg, registerTerminal, terminalOf } from '../terminals'
import { toast } from '../toasts'
import { closeWin, focusWin, openWin, winById } from '../windows'
import { Icon, type IconName } from './Icon'
import { useXterm } from './useXterm'

// コマンドの行を、先頭の sudo を飛ばした語の並びにする。-u ユーザーのように値を取るオプションは値も飛ばす
function words(cmdline: string) {
  const w = cmdline.trim().split(/\s+/)
  if (w[0]?.split('/').pop() === 'sudo') {
    w.shift()
    while (w[0]?.startsWith('-')) {
      if (['-u', '-g', '-h', '-p', '-C', '-D', '-r', '-t', '-U'].includes(w.shift()!)) w.shift()
    }
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

// ターミナルの見出しから、移した先のウィンドウへ影を飛ばす (mock/ と同じ動き)
function flyGhost(from: DOMRect, winId: string, label: string) {
  const to = document.querySelector(`[data-win="${winId}"]`)?.getBoundingClientRect()
  if (!to?.width || !from.width) return
  const g = document.createElement('div')
  g.className = 'fly-ghost'
  g.textContent = label
  document.body.append(g)
  g.animate([
    { left: `${from.left}px`, top: `${from.top - 4}px`, width: `${Math.min(from.width, 420)}px`, height: '30px', opacity: 1 },
    { left: `${to.left}px`, top: `${to.top}px`, width: `${to.width}px`, height: `${to.height}px`, opacity: 0 },
  ], { duration: 460, easing: 'cubic-bezier(.2,.7,.2,1)' }).onfinish = () => g.remove()
}

// ウィンドウに移したセッションのアイコン。コマンドの種類で分ける
function iconOf(cmdline: string): IconName {
  const cmd = words(cmdline)[0]
  return cmd === 'ssh' ? 'link' : cmd === 'docker' ? 'box' : cmd === 'mysql' || cmd === 'psql' ? 'db'
    : cmd === 'top' || cmd === 'htop' ? 'chart' : cmd === 'tail' || cmd === 'journalctl' ? 'log'
      : cmd === 'screen' || cmd === 'tmux' ? 'layers' : 'terminal'
}

// 右下のターミナル。サーバーごとに1つシェルを開く。
// generation は再接続のたびに増える。増えたら同じ画面のまま新しいシェルを開き直す。
// 前面で動いているコマンドは見出しに出し、ドラッグかクリックでデスクトップのウィンドウに移す
export function TerminalPanel({ serverId, home, generation, visible, fontSize, onForegroundChange }: {
  serverId: string
  home: string
  generation: number
  visible: boolean
  fontSize: number
  onForegroundChange: (cmdline: string) => void
}) {
  const { t } = useTranslation()
  const settings = useSettings()
  const mobile = useMobile()
  const host = useRef<HTMLDivElement>(null)
  const { termRef, fitRef } = useXterm(host, fontSize)
  const shellRef = useRef('')
  const detachRef = useRef(() => {})
  const [fg, setFgState] = useState('')
  const [dropLabel, setDropLabel] = useState('')
  // スマホの Ctrl。押したあとに打った1文字を Ctrl 付きにする
  const [ctrl, setCtrlState] = useState(false)
  const ctrlRef = useRef(false)
  const setCtrl = (v: boolean) => {
    ctrlRef.current = v
    setCtrlState(v)
  }
  // ターミナルで開くセッションは、自動でウィンドウへ移さない
  const noAutoRef = useRef(false)
  const fgChangeRef = useRef(onForegroundChange)
  fgChangeRef.current = onForegroundChange
  const setFg = (c: string) => {
    setFgState(c)
    fgChangeRef.current(c)
  }
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
    const onData = term.onData(d => {
      if (ctrlRef.current && d.length === 1 && /[@-~a-z]/i.test(d)) {
        d = String.fromCharCode(d.toUpperCase().charCodeAt(0) & 0x1f)
        setCtrl(false)
      }
      if (shellRef.current) sendInput(shellRef.current, d)
    })
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
    const from = document.querySelector(`[data-terminal="${serverId}"] [role=button][draggable]`)?.getBoundingClientRect()
    detachRef.current()
    shellRef.current = ''
    setFg('')
    dim(tRef.current('desktop.moved', { cmd }))
    const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
    const winId = openWin({
      serverId, kind: 'session', title: cmd, icon: iconOf(cmd), shellId: id,
      detachable: isDetachable(cmd), replayFromIdle: true, rect: pos, sessionKey: sessionKeyOf(cmd) || undefined,
    }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
    // スマホでターミナルを見ているときは、移したことを知らせ、デスクトップへ行けるようにする。
    // PC では、ターミナルの見出しからウィンドウへ影を飛ばす
    if (isMobile()) {
      if (getMTab() !== 'desktop') toast(tRef.current('desktop.moved', { cmd }), 'info', { label: tRef.current('desktop.open'), fn: () => { setMTab('desktop'); focusWin(winId) } })
    } else if (from) requestAnimationFrame(() => flyGhost(from, winId, cmd))
    open(await ShellCwd(id).catch(() => ''))
  }
  const popOutRef = useRef(popOut)
  popOutRef.current = popOut

  useEffect(() => onForeground((id, cmdline) => {
    if (id !== shellRef.current) return
    setFg(cmdline)
    const s = settingsRef.current
    if (cmdline && noAutoRef.current) noAutoRef.current = false
    else if (cmdline && s.autoOn && matchesAuto(cmdline, s.autoCmds)) popOutRef.current()
  }), [])

  // ウィンドウのセッションをターミナルに戻す。今のシェルは何も動いていなければ閉じ、ウィンドウのチャネルに付け替える
  const takeBack = async (winId: string) => {
    const w = winById(winId)
    const term = termRef.current
    if (!w || w.ended || !term) return
    if (shellRef.current && foregroundOf(shellRef.current)) {
      toast(tRef.current('toast.busy'), 'warn')
      return
    }
    detachRef.current()
    if (shellRef.current) CloseShell(shellRef.current)
    closeWin(w.id)
    term.reset()
    const r = await ShellReplay(w.shellId, false)
    term.write(decode(r.data))
    shellRef.current = w.shellId
    detachRef.current = attachShell(w.shellId, {
      out: data => term.write(data),
      exit: () => {
        if (shellRef.current === w.shellId) shellRef.current = ''
        dim(tRef.current('terminal.ended'))
      },
    }, r.end)
    setFg(foregroundOf(w.shellId))
    ShellResize(w.shellId, term.cols, term.rows)
    dim(tRef.current('desktop.returned', { cmd: w.title }))
    term.focus()
  }
  const takeBackRef = useRef(takeBack)
  takeBackRef.current = takeBack

  // セッションの一覧からドラッグしてきたものを、ここで開く
  const openSession = (s: { kind: string; name: string }) => {
    const id = shellRef.current
    if (!id) return
    if (foregroundOf(id)) {
      toast(tRef.current('toast.busy'), 'warn')
      return
    }
    if (!typeLine(id, attachCommand(s), true)) return
    noAutoRef.current = true
    termRef.current?.focus()
  }

  // ファイルツリーから cd やパスを送る窓口。前面で別のコマンドが動いているときは送らない
  useEffect(() => {
    const send = (text: string, enter: boolean) => {
      const id = shellRef.current
      if (!id) return
      if (foregroundOf(id)) {
        toast(tRef.current('toast.busy'), 'warn')
        return
      }
      if (typeLine(id, text, enter)) termRef.current?.focus()
    }
    return registerTerminal(serverId, {
      cd: path => send(`cd ${path}`, true),
      insert: text => send(text, false),
      popOut: pos => popOutRef.current(pos),
      takeBack: winId => takeBackRef.current(winId),
      setDropLabel,
      foreground: () => shellRef.current ? foregroundOf(shellRef.current) : '',
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
    <section
      data-terminal={serverId}
      className={`relative flex min-h-0 flex-col bg-term ${mobile ? 'flex-1' : 'term-card mx-2 mb-2'}`}
      onDragOver={e => {
        const types = e.dataTransfer.types
        if (types.includes('application/x-drag-session')) setDropLabel(t('desktop.openHere'))
        else if (types.includes('application/x-drag-path')) setDropLabel(types.includes('application/x-drag-dir') ? t('open.cdHere') : t('open.insertHere'))
        else return
        e.preventDefault()
      }}
      onDragLeave={() => setDropLabel('')}
      onDrop={e => {
        setDropLabel('')
        const data = e.dataTransfer.getData('application/x-drag-session')
        const file = e.dataTransfer.getData('application/x-drag-path')
        if (data) {
          e.preventDefault()
          openSession(JSON.parse(data) as remote.Session)
        } else if (file) {
          // ツリーのフォルダなら cd、ファイルならパスを入力する
          e.preventDefault()
          const f = JSON.parse(file) as { path: string; dir: boolean }
          const api = terminalOf(serverId)
          if (f.dir) api?.cd(pathArg(home, f.path))
          else api?.insert(pathArg(home, f.path))
        }
      }}
    >
      <div className="flex h-[38px] flex-none items-center gap-2.5 bg-term-head pr-2 pl-4 shadow-[inset_0_-1px_0_var(--term-line)]">
        <span className="eyebrow term-title">{t('terminal.title')}</span>
        {fg && (
          <div
            draggable
            role="button"
            className="fg-chip inline-flex h-[26px] min-w-0 cursor-grab items-center gap-2 pr-3 pl-1.5 select-none active:cursor-grabbing"
            onDragStart={e => {
              e.dataTransfer.setData('application/x-drag-fg', serverId)
              e.dataTransfer.effectAllowed = 'move'
            }}
            onClick={() => {
              popOut()
              setMTab('desktop')
            }}
            title={fg}
          >
            <Icon name="grip" className="grip text-term-dir" />
            <span className="max-w-[260px] truncate font-mono text-[12px]">{fg}</span>
            <span className="text-[11px] whitespace-nowrap text-term-dim">{mobile ? t('desktop.chipHintMobile') : t('desktop.chipHint')}</span>
          </div>
        )}
        <div className="flex-1" />
        <button className="icon-btn h-[26px] w-[26px] !text-term-dim hover:!bg-[color-mix(in_srgb,var(--term-fg)_10%,transparent)] hover:!text-term-strong" title={t('terminal.clear')} onClick={() => termRef.current?.clear()}><Icon name="eraser" /></button>
      </div>
      <div ref={host} className="min-h-0 flex-1" />
      {mobile && <ExtraKeys ctrl={ctrl} onCtrl={() => setCtrl(!ctrl)} onKey={seq => termRef.current?.input(seq, false)} arrow={k => (termRef.current?.modes.applicationCursorKeysMode ? '\x1bO' : '\x1b[') + k} />}
      {dropLabel && (
        <div className="drop-zone pointer-events-none absolute inset-2 z-10 grid place-items-center">
          <span className="drop-tag">{dropLabel}</span>
        </div>
      )}
    </section>
  )
}

// スマホのキーボードにないキーの列。押してもキーボードを閉じないよう、フォーカスを動かさない
function ExtraKeys({ ctrl, onCtrl, onKey, arrow }: { ctrl: boolean; onCtrl: () => void; onKey: (seq: string) => void; arrow: (k: string) => string }) {
  const keys: [string, () => void][] = [
    ['Esc', () => onKey('\x1b')], ['Tab', () => onKey('\t')], ['Ctrl', onCtrl],
    ['↑', () => onKey(arrow('A'))], ['↓', () => onKey(arrow('B'))], ['←', () => onKey(arrow('D'))], ['→', () => onKey(arrow('C'))],
    ['|', () => onKey('|')], ['/', () => onKey('/')], ['~', () => onKey('~')], ['-', () => onKey('-')],
  ]
  return (
    <div className="flex flex-none gap-1.5 overflow-x-auto bg-term-head px-2 py-[7px] shadow-[inset_0_1px_0_var(--term-line)] [scrollbar-width:none]">
      {keys.map(([label, fn]) => (
        <button
          key={label}
          data-key={label}
          className={`xkey h-9 min-w-[42px] flex-none px-3 font-mono text-[13px] ${label === 'Ctrl' && ctrl ? 'on' : ''}`}
          onPointerDown={e => e.preventDefault()}
          onClick={fn}
        >{label}</button>
      ))}
    </div>
  )
}
