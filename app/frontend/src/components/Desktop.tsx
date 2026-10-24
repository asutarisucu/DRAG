import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloseShell, OpenShell } from '../../wailsjs/go/main/App'
import { openFile } from '../openFile'
import { openSessionWindow } from '../openSession'
import { ArchiveView, EditorView, ImageView, LogView } from './FileViews'
import { MonitorView } from './Monitor'
import { openTool, TOOLS } from '../tools'
import { onForeground } from '../shellBus'
import { useMobile } from '../mobile'
import { useSettings } from '../settings'
import { terminalOf } from '../terminals'
import { toast } from '../toasts'
import { canClose, closeWin, focusWin, openWin, reorderWin, topWin, updateWin, useWins, winById, winsOf, type Win } from '../windows'
import { Icon } from './Icon'
import { ShellView } from './ShellView'
import { Window, type SnapMode } from './Window'

// 右上のデスクトップ。ウィンドウを重ねて置き、下のタスクバーで切り替える
export function Desktop({ serverId, home }: { serverId: string; home: string }) {
  const { t } = useTranslation()
  const settings = useSettings()
  const mobile = useMobile()
  const wins = useWins(serverId)
  const layer = useRef<HTMLDivElement>(null)
  const [snap, setSnap] = useState<SnapMode>(null)
  const [dropOn, setDropOn] = useState(false)
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

  const close = async (w: Win) => {
    if (!(await canClose(w.id))) return
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
          if (!['application/x-drag-fg', 'application/x-drag-session', 'application/x-drag-path'].some(x => types.includes(x))) return
          e.preventDefault()
          setDropOn(true)
        }}
        onDragLeave={() => setDropOn(false)}
        onDrop={e => {
          setDropOn(false)
          const r = host()
          const pos = { x: e.clientX - r.left - 40, y: e.clientY - r.top - 16 }
          const session = e.dataTransfer.getData('application/x-drag-session')
          const file = e.dataTransfer.getData('application/x-drag-path')
          if (file) {
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
        {wins.length === 0 && <div className="pointer-events-none absolute inset-0 grid place-items-center px-6 text-center text-faint select-none">{mobile ? t('desktop.emptyMobile') : t('desktop.empty')}</div>}
        {wins.map(w => (
          <Window key={w.id} win={w} focused={w.id === focused} host={host} onSnapPreview={setSnap} onMinimize={() => minimize(w)} onClose={() => close(w)}>
            {w.kind === 'editor' ? <EditorView win={w} serverId={serverId} home={home} />
              : w.kind === 'log' ? <LogView win={w} serverId={serverId} home={home} />
                : w.kind === 'image' ? <ImageView win={w} serverId={serverId} />
                  : w.kind === 'archive' ? <ArchiveView win={w} serverId={serverId} home={home} />
                    : w.kind === 'monitor' ? <MonitorView win={w} serverId={serverId} home={home} />
                    : <ShellView shellId={w.shellId} fromIdle={w.replayFromIdle} fontSize={settings.termSize} focused={w.id === focused} shared={w.shared} onExit={() => { if (!winById(w.id)?.ended) closeWin(w.id) }} />}
          </Window>
        ))}
        {preview && <div className="pointer-events-none absolute z-[2147483000] rounded-xl border border-[color-mix(in_srgb,var(--accent)_60%,transparent)] bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] backdrop-blur-[3px]" style={preview} />}
        {dropOn && (
          <div className="pointer-events-none absolute inset-2 z-[2147483001] grid place-items-center rounded-xl border-2 border-dashed border-[color-mix(in_srgb,var(--accent)_75%,transparent)] bg-[color-mix(in_srgb,var(--accent)_9%,transparent)] text-[13px] font-semibold">
            <span className="rounded-lg bg-raised px-3 py-1.5 shadow-win">{t('desktop.dropHere')}</span>
          </div>
        )}
      </div>
      <Taskbar serverId={serverId} wins={wins} focused={focused} onNewTerminal={newTerminal} onTile={tile} onClose={close} onMinimize={minimize} />
    </section>
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
    <footer className={`flex flex-none items-center gap-1 border-line bg-[color-mix(in_srgb,var(--chrome)_90%,transparent)] px-2 backdrop-blur-md ${mobile ? 'order-first h-[46px] border-b' : 'h-[42px] border-t'}`}>
      <button className="grid h-8 w-[34px] place-items-center rounded-[7px] text-muted hover:bg-[color-mix(in_srgb,var(--text)_6%,transparent)] hover:text-text" title={t('desktop.newTerminal')} onClick={onNewTerminal}>
        <Icon name="terminal" size={18} />
      </button>
      {TOOLS.map(([k, icon]) => (
        <button key={k} data-tool={k} className="grid h-8 w-[34px] place-items-center rounded-[7px] text-muted hover:bg-[color-mix(in_srgb,var(--text)_6%,transparent)] hover:text-text" title={t(`tools.${k}`)} onClick={() => openTool(serverId, k)}>
          <Icon name={icon} size={18} />
        </button>
      ))}
      <span className="mx-1.5 h-[22px] w-px flex-none bg-line-strong" />
      <div ref={box} className="flex min-w-0 flex-1 gap-1 overflow-x-auto [scrollbar-width:none]">
        {wins.map((w, i) => {
          const active = w.id === focused && !w.min
          return (
            <div
              key={w.id}
              role="button"
              title={w.title}
              data-task={w.id}
              className={`group relative flex h-8 max-w-[220px] flex-none cursor-pointer items-center gap-[7px] rounded-[7px] pr-[5px] pl-3 select-none transition-[background,color] after:absolute after:bottom-0.5 after:left-1/2 after:h-[3px] after:-translate-x-1/2 after:rounded-sm ${active ? 'bg-[color-mix(in_srgb,var(--text)_8%,transparent)] text-text after:w-4 after:bg-accent' : 'text-muted after:w-1.5 after:bg-faint hover:bg-[color-mix(in_srgb,var(--text)_5%,transparent)] hover:text-text'} ${w.min ? 'opacity-60' : ''}`}
              onClick={() => click(w)}
              onPointerDown={e => onDown(e, i)}
            >
              <Icon name={w.icon} />
              <span className="truncate text-[12px]">{w.title}</span>
              <button className="grid h-5 w-5 flex-none place-items-center rounded-[5px] text-muted opacity-45 group-hover:opacity-90 hover:bg-[color-mix(in_srgb,var(--text)_12%,transparent)] hover:text-text" title={t('desktop.close')} onClick={e => { e.stopPropagation(); onClose(w) }}>
                <Icon name="x" size={12} />
              </button>
            </div>
          )
        })}
      </div>
      {!mobile && <button className="grid h-7 w-7 place-items-center rounded-md text-muted hover:text-text" title={t('desktop.tile')} onClick={onTile}><Icon name="grid" /></button>}
    </footer>
  )
}
