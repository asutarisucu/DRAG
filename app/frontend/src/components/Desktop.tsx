import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloseShell, OpenShell } from '../../wailsjs/go/main/App'
import { openFile } from '../openFile'
import { openSessionWindow } from '../openSession'
import { attachCommand } from '../sessions'
import { sendInput } from '../shellInput'
import { ArchiveView, EditorView, ImageView, LogView } from './FileViews'
import { DockerView } from './DockerView'
import { MonitorView } from './Monitor'
import { ServicesView } from './Services'
import { TrashView } from './TrashView'
import { openContainerShell, openTool, TOOLS } from '../tools'
import { onForeground } from '../shellBus'
import { useMobile } from '../mobile'
import { enterTask, slideMark, slideX, useSlide } from '../motion'
import { useSettings } from '../settings'
import { terminalOf } from '../terminals'
import { toast } from '../toasts'
import { canClose, closeWin, focusWin, openWin, reorderWin, topWin, updateWin, useWins, winById, winsOf, type Win } from '../windows'
import { Icon } from './Icon'
import { ShellView } from './ShellView'
import { Window, type SnapMode } from './Window'

// 右上のデスクトップ。ウィンドウを重ねて置き、下のタスクバーで切り替える
// generation は再接続のたびに増える。増えたら、接続が切れて終わったウィンドウのシェルを開き直す
export function Desktop({ serverId, home, generation }: { serverId: string; home: string; generation: number }) {
  const { t } = useTranslation()
  const settings = useSettings()
  const mobile = useMobile()
  const wins = useWins(serverId)
  const layer = useRef<HTMLDivElement>(null)
  const [snap, setSnap] = useState<SnapMode>(null)
  // デスクトップへドラッグしている間に出す案内。空なら出さない
  const [dropOn, setDropOn] = useState('')
  const termCount = useRef(0)
  const host = () => layer.current!.getBoundingClientRect()
  const focused = topWin(serverId)?.id

  // セッションのコマンドが終わってシェルに戻ったら、screen と tmux はデタッチしたものとして閉じ、
  // それ以外は「終了」と出してシェルを閉じる
  useEffect(() => onForeground((shellId, cmdline) => {
    if (cmdline) return
    // 描画の時点の wins ではなく、最新の一覧を見る
    const w = winsOf(serverId).find(x => x.shellId === shellId && x.kind === 'session' && !x.ended)
    if (!w) return
    CloseShell(shellId)
    if (w.detachable) {
      closeWin(w.id)
      toast(t('desktop.detached', { title: w.title }))
    } else {
      updateWin(w.id, { ended: true, title: t('desktop.ended', { title: w.title }) })
    }
  }), [serverId, t])

  // シェルが終わったら、ふつうはウィンドウを閉じる。接続が切れて終わったときは、
  // ターミナルと screen・tmux のウィンドウは残して再接続を待ち、ほかのセッションは「終了」と出して残す
  const exited = (id: string, lost: boolean) => {
    const w = winById(id)
    if (!w || w.ended) return
    if (!lost) closeWin(id)
    else if (w.kind === 'terminal' || (w.detachable && w.sessionKey)) updateWin(id, { lost: true })
    else updateWin(id, { ended: true, title: t('desktop.ended', { title: w.title }) })
  }
  useEffect(() => {
    if (generation === 0) return
    for (const w of winsOf(serverId)) {
      if (!w.lost) continue
      OpenShell(serverId, 100, 30, '').then(id => {
        if (!winById(w.id)) return CloseShell(id)
        updateWin(w.id, { shellId: id, lost: false, replayFromIdle: false })
        if (w.kind === 'session' && w.sessionKey) {
          const i = w.sessionKey.indexOf(':')
          sendInput(id, attachCommand({ kind: w.sessionKey.slice(0, i), name: w.sessionKey.slice(i + 1) }) + '\r')
        }
      }, e => toast(String(e), 'warn'))
    }
  }, [generation])

  // 閉じるときは、ウィンドウの写しを薄くしながら消す。本体はすぐに閉じる (mock/ と同じ動き)
  const fade = (id: string) => {
    const node = layer.current?.querySelector<HTMLElement>(`[data-win="${id}"]`)
    if (mobile || !node) return
    const ghost = node.cloneNode(true) as HTMLElement
    ghost.removeAttribute('data-win')
    ghost.style.pointerEvents = 'none'
    layer.current!.append(ghost)
    ghost.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.96)' }], { duration: 120, easing: 'ease-in', fill: 'forwards' }).onfinish = () => ghost.remove()
  }

  const close = async (w: Win) => {
    if (!(await canClose(w.id))) return
    fade(w.id)
    if (w.shellId && !w.shared) CloseShell(w.shellId)
    if (w.detachable && !w.ended) toast(t('desktop.detached', { title: w.title }))
    closeWin(w.id)
  }
  const minimize = (w: Win) => updateWin(w.id, { min: true })

  const newTerminal = async () => {
    const cwd = await terminalOf(serverId)?.cwd().catch(() => '') ?? ''
    const id = await OpenShell(serverId, 80, 24, cwd).catch(e => { toast(String(e), 'warn'); return '' })
    if (!id) return
    termCount.current++
    openWin({ serverId, kind: 'terminal', title: t('desktop.terminalN', { n: termCount.current }), icon: 'terminal', shellId: id, detachable: false, replayFromIdle: false, rect: { w: 520, h: 300 } }, size())
  }
  const size = () => {
    const r = host()
    return { w: r.width, h: r.height }
  }

  const tile = () => {
    const ws = wins.filter(w => !w.min)
    if (!ws.length) return
    const H = host(), g = 8
    const cols = Math.ceil(Math.sqrt(ws.length)), rows = Math.ceil(ws.length / cols)
    const cw = (H.width - g * (cols + 1)) / cols, ch = (H.height - g * (rows + 1)) / rows
    ws.forEach((w, i) => updateWin(w.id, { max: false, snapped: false, rect: { x: g + (i % cols) * (cw + g), y: g + Math.floor(i / cols) * (ch + g), w: cw, h: ch } }))
  }

  const preview = snap && layer.current && (() => {
    const H = host()
    const r = snap === 'left' ? [0, 0, H.width / 2, H.height] : snap === 'right' ? [H.width / 2, 0, H.width / 2, H.height] : [0, 0, H.width, H.height]
    return { left: r[0] + 6, top: r[1] + 6, width: r[2] - 12, height: r[3] - 12 }
  })()

  return (
    <section className="relative flex min-h-0 min-w-0 flex-1 flex-col">
      <div
        ref={layer}
        data-desktop={serverId}
        className="desk-bg relative isolate flex-1 overflow-hidden"
        onDragOver={e => {
          const types = e.dataTransfer.types
          const label = types.includes('application/x-drag-fg') ? t('desktop.dropHere')
            : types.includes('application/x-drag-container') ? t('desktop.dropShell')
              : types.includes('application/x-drag-session') ? t('desktop.dropOpen')
                : types.includes('application/x-drag-path') && !types.includes('application/x-drag-dir') && !types.includes('application/x-drag-bin') ? t('desktop.dropOpen')
                  : ''
          if (!label) return
          e.preventDefault()
          setDropOn(label)
        }}
        onDragLeave={() => setDropOn('')}
        onDrop={e => {
          setDropOn('')
          const r = host()
          const pos = { x: e.clientX - r.left - 40, y: e.clientY - r.top - 16 }
          const session = e.dataTransfer.getData('application/x-drag-session')
          const file = e.dataTransfer.getData('application/x-drag-path')
          const container = e.dataTransfer.getData('application/x-drag-container')
          if (container) {
            e.preventDefault()
            openContainerShell(serverId, container, pos)
          } else if (file) {
            e.preventDefault()
            openFile(serverId, JSON.parse(file), pos)
          } else if (session) {
            e.preventDefault()
            openSessionWindow(serverId, JSON.parse(session), pos)
          } else if (e.dataTransfer.types.includes('application/x-drag-fg')) {
            e.preventDefault()
            terminalOf(serverId)?.popOut(pos)
          }
        }}
      >
        {wins.length === 0 && (
          <div className="pointer-events-none absolute inset-0 grid place-content-center justify-items-center gap-4 px-6 text-center text-muted select-none">
            {!mobile && <EmptyHint />}
            {mobile ? t('desktop.emptyMobile') : t('desktop.empty')}
          </div>
        )}
        {wins.map(w => (
          <Window key={w.id} win={w} focused={w.id === focused} host={host} onSnapPreview={setSnap} onMinimize={() => minimize(w)} onClose={() => close(w)}>
            {w.kind === 'editor' ? <EditorView win={w} serverId={serverId} home={home} />
              : w.kind === 'log' ? <LogView win={w} serverId={serverId} home={home} />
                : w.kind === 'image' ? <ImageView win={w} serverId={serverId} />
                  : w.kind === 'archive' ? <ArchiveView win={w} serverId={serverId} home={home} />
                    : w.kind === 'monitor' ? <MonitorView win={w} serverId={serverId} home={home} />
                    : w.kind === 'services' ? <ServicesView serverId={serverId} />
                    : w.kind === 'docker' ? <DockerView serverId={serverId} home={home} />
                    : w.kind === 'trash' ? <TrashView serverId={serverId} home={home} />
                    : <ShellView
                      shellId={w.shellId} fromIdle={w.replayFromIdle} fontSize={settings.termSize} focused={w.id === focused} shared={w.shared}
                      lostText={w.kind === 'terminal' ? t('desktop.lostTerminal') : w.detachable && w.sessionKey ? t('desktop.lostSession') : t('desktop.lostEnded')}
                      reopenText={w.kind === 'terminal' ? t('terminal.reopened') : t('desktop.reattached')}
                      onExit={lost => exited(w.id, lost)}
                    />}
          </Window>
        ))}
        {preview && <div className="snap-preview pointer-events-none absolute z-[2147483000]" style={preview} />}
        {/* 落とせる場所は、点線の枠が回る。何が起きるかは真ん中の札に出す */}
        {dropOn && (
          <div className="drop-zone pointer-events-none absolute inset-2 z-[2147483001] grid place-items-center">
            <span className="drop-tag">{dropOn}</span>
          </div>
        )}
      </div>
      <Taskbar serverId={serverId} wins={wins} focused={focused} onNewTerminal={newTerminal} onTile={tile} onClose={close} onMinimize={minimize} />
    </section>
  )
}

// 空のデスクトップでは、ファイルを窓の枠へドラッグする動きを繰り返して見せる
function EmptyHint() {
  return (
    <div className="empty-hint" aria-hidden="true">
      <span className="eh-frame" />
      <span className="eh-file"><Icon name="log" size={14} />latest.log</span>
      <svg className="eh-cursor" viewBox="0 0 24 24"><path d="M5 3l14 8.2-6.1 1.3 3.6 6.6-2.7 1.5-3.6-6.6L5.6 18z" /></svg>
    </div>
  )
}

function Taskbar({ serverId, wins, focused, onNewTerminal, onTile, onClose, onMinimize }: {
  serverId: string
  wins: Win[]
  focused?: string
  onNewTerminal: () => void
  onTile: () => void
  onClose: (w: Win) => void
  onMinimize: (w: Win) => void
}) {
  const { t } = useTranslation()
  const mobile = useMobile()
  const box = useRef<HTMLDivElement>(null)
  const dragged = useRef(false)
  // 選んでいる項目の下地と下の印を、前の項目から滑らせる
  useSlide(box, wins.find(w => w.id === focused && !w.min)?.id, (el, from, to) => {
    slideX('::before')(el, from, to)
    slideMark(el, from, to)
  })
  // 前面のウィンドウの項目が見えるようにスクロールする
  useEffect(() => {
    if (focused) box.current?.querySelector(`[data-task="${focused}"]`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [focused])

  // スマホではウィンドウを1つずつ出すので、押したものを前に出すだけにする
  const click = (w: Win) => {
    if (dragged.current) return
    if (w.min || mobile) focusWin(w.id)
    else if (w.id === focused) onMinimize(w)
    else focusWin(w.id)
  }

  // 下のタブはドラッグで並べ替える (mock/ と同じ動き)
  const onDown = (e: React.PointerEvent, idx: number) => {
    if (mobile || e.button !== 0 || (e.target as HTMLElement).closest('button')) return
    const item = e.currentTarget as HTMLElement
    const items = [...box.current!.children] as HTMLElement[]
    const rects = items.map(i => i.getBoundingClientRect())
    const startX = e.clientX
    let moved = false, target = idx
    item.setPointerCapture(e.pointerId)
    const move = (ev: PointerEvent) => {
      const dx = ev.clientX - startX
      if (!moved) {
        if (Math.abs(dx) < 5) return
        moved = true
        item.style.zIndex = '2'
      }
      const cdx = Math.max(rects[0].left - rects[idx].left, Math.min(rects.at(-1)!.right - rects[idx].right, dx))
      item.style.transform = `translateX(${cdx}px)`
      // 動かす向きの端が、ほかのタブの真ん中を越えたら入れ替える。
      // 真ん中どうしで比べると、幅の広いタブを狭いタブの後ろへ動かせない
      const mid = (r: DOMRect) => r.left + r.width / 2
      target = cdx > 0
        ? idx + rects.filter((r, i) => i > idx && mid(r) < rects[idx].right + cdx).length
        : idx - rects.filter((r, i) => i < idx && mid(r) > rects[idx].left + cdx).length
      const shift = rects[idx].width + 4
      items.forEach((it, i) => {
        if (i === idx) return
        const s = idx < target && i > idx && i <= target ? -shift : idx > target && i >= target && i < idx ? shift : 0
        it.style.transform = s ? `translateX(${s}px)` : ''
      })
    }
    const up = () => {
      item.removeEventListener('pointermove', move)
      item.removeEventListener('pointerup', up)
      item.removeEventListener('pointercancel', up)
      items.forEach(it => { it.style.transform = ''; it.style.zIndex = '' })
      if (!moved) return
      dragged.current = true
      setTimeout(() => { dragged.current = false }, 0)
      reorderWin(serverId, idx, target)
    }
    item.addEventListener('pointermove', move)
    item.addEventListener('pointerup', up)
    item.addEventListener('pointercancel', up)
  }

  return (
    <footer className={`taskbar flex flex-none items-center gap-1 px-2.5 ${mobile ? 'order-first h-[50px] shadow-[inset_0_-1px_0_var(--border-soft)]' : 'h-12 shadow-[inset_0_1px_0_var(--border-soft)]'}`}>
      <button className="launch-btn" title={t('desktop.newTerminal')} onClick={onNewTerminal}>
        <Icon name="terminal" size={18} />
      </button>
      {TOOLS.map(([k, icon]) => (
        <button key={k} data-tool={k} className="launch-btn" title={t(`tools.${k}`)} onClick={() => openTool(serverId, k)}>
          <Icon name={icon} size={18} />
        </button>
      ))}
      <span className="mx-1.5 h-[22px] w-px flex-none bg-line" />
      <div ref={box} className="flex min-w-0 flex-1 gap-1 overflow-x-auto px-0.5 py-1 [scrollbar-width:none]">
        {wins.map((w, i) => {
          const active = w.id === focused && !w.min
          return (
            <div
              key={w.id}
              ref={enterTask}
              role="button"
              title={w.title}
              data-task={w.id}
              data-slide={w.id}
              className={`task-item group flex h-[34px] max-w-[220px] flex-none cursor-pointer items-center gap-[7px] pr-[5px] pl-[13px] select-none ${active ? 'active' : ''} ${w.min ? 'minimized' : ''}`}
              onClick={() => click(w)}
              onPointerDown={e => onDown(e, i)}
            >
              <Icon name={w.icon} />
              <span className="truncate text-[12px]">{w.title}</span>
              <button className={`icon-btn h-5 w-5 group-hover:opacity-90 hover:!text-err ${active ? 'opacity-90' : 'opacity-45'}`} title={t('desktop.close')} onClick={e => { e.stopPropagation(); onClose(w) }}>
                <Icon name="x" size={12} />
              </button>
            </div>
          )
        })}
      </div>
      {!mobile && <button className="icon-btn h-7 w-7" title={t('desktop.tile')} onClick={onTile}><Icon name="grid" /></button>}
    </footer>
  )
}
