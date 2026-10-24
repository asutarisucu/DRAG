import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArchiveClose, ArchiveOpen, ArchiveRead, LogFollow, LogStop, LogTail, OpenText, ReadImage, SaveText } from '../../wailsjs/go/main/App'
import type { archive } from '../../wailsjs/go/models'
import { KIND_ICON, fileKind, logParts } from '../fileKind'
import { attachLog } from '../logBus'
import { useSettings } from '../settings'
import { fmtSize } from '../toasts'
import { setCloseGuard, updateWin, type Win } from '../windows'
import { Icon } from './Icon'
import { Modal } from './Modal'

const errText = (e: unknown) => String(e).replace(/^Error: /, '')
const toolbar = 'flex h-[34px] flex-none items-center gap-2 border-b border-line bg-panel pr-2 pl-2.5 text-[12px]'
const pathCls = 'min-w-0 truncate font-mono text-[11.5px] text-muted'
const smallBtn = 'h-7 rounded-lg border border-line-strong bg-raised px-2.5 text-[12px] hover:bg-btn-hover disabled:opacity-45'
const baseName = (p: string) => p.slice(p.lastIndexOf('/') + 1)

function Failed({ text }: { text: string }) {
  return <div className="grid flex-1 place-items-center bg-term p-4 text-center text-[12.5px] text-muted">{text}</div>
}

// 行番号と本文を並べる。読み取り専用の表示とエディターで使う
function Gutter({ lines, scrollTop }: { lines: number; scrollTop: number }) {
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => { if (ref.current) ref.current.scrollTop = scrollTop }, [scrollTop])
  return (
    <div ref={ref} className="w-11 flex-none overflow-hidden border-r border-[color-mix(in_srgb,#cfd6e0_15%,transparent)] py-2 pr-2.5 text-right font-mono text-[12.5px] leading-[1.5] whitespace-pre text-[#5c6779] select-none">
      {Array.from({ length: lines }, (_, i) => i + 1).join('\n')}
    </div>
  )
}

// エディター。Ctrl+S で保存し、開いたあとにサーバーで変わっていたら上書きするかを聞く
export function EditorView({ win, serverId, home }: { win: Win; serverId: string; home: string }) {
  const { t } = useTranslation()
  const path = win.path!
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [dirty, setDirty] = useState(false)
  const [status, setStatus] = useState<'' | 'saved'>('')
  const [scroll, setScroll] = useState(0)
  const [ask, setAsk] = useState<null | 'changed' | 'unsaved'>(null)
  const mod = useRef(0)
  const textRef = useRef('')
  // 確認の窓の答え。保存して閉じるときは 'save'
  const answer = useRef<(v: boolean | 'save') => void>(() => {})
  const ta = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    OpenText(serverId, path).then(f => {
      setText(f.text)
      textRef.current = f.text
      mod.current = f.modTime
    }, e => setError(errText(e)))
  }, [serverId, path])

  useEffect(() => { updateWin(win.id, { title: (dirty ? '● ' : '') + baseName(path) }) }, [dirty, win.id, path])

  const save = async (force = false): Promise<boolean> => {
    try {
      mod.current = await SaveText(serverId, path, textRef.current, mod.current, force)
      setDirty(false)
      setStatus('saved')
      setTimeout(() => setStatus(''), 1800)
      return true
    } catch (e) {
      if (!force && errText(e).includes('書き換えられています')) {
        setAsk('changed')
        const ok = await new Promise<boolean | 'save'>(r => { answer.current = r })
        setAsk(null)
        return ok === true ? save(true) : false
      }
      setError(errText(e))
      return false
    }
  }

  // 保存していなければ、閉じる前に聞く
  useEffect(() => {
    setCloseGuard(win.id, dirty ? async () => {
      setAsk('unsaved')
      const choice = await new Promise<boolean | 'save'>(r => { answer.current = r })
      setAsk(null)
      if (choice === 'save') return save()
      return choice === true
    } : null)
    return () => setCloseGuard(win.id, null)
  })

  if (error && text === null) return <Failed text={error} />
  const lines = (text ?? '').split('\n').length
  return (
    <>
      <div className={toolbar}>
        <span className={pathCls}>{path.startsWith(home + '/') ? '~' + path.slice(home.length) : path}</span>
        <div className="flex-1" />
        <span className={`text-[11.5px] whitespace-nowrap ${status ? 'text-ok' : 'text-muted'}`}>{status ? t('editor.saved') : error || t('editor.lines', { n: lines })}</span>
        <button className={smallBtn} disabled={text === null} onClick={() => save()}>{t('editor.save')}</button>
      </div>
      <div className="flex min-h-0 flex-1 bg-term">
        <Gutter lines={lines} scrollTop={scroll} />
        <textarea
          ref={ta}
          className="m-0 flex-1 resize-none overflow-auto border-0 bg-transparent px-3 py-2 font-mono text-[12.5px] leading-[1.5] whitespace-pre text-[#eef2f8] outline-none [tab-size:2]"
          spellCheck={false}
          wrap="off"
          value={text ?? ''}
          readOnly={text === null}
          onChange={e => { setText(e.target.value); textRef.current = e.target.value; setDirty(true) }}
          onScroll={e => setScroll(e.currentTarget.scrollTop)}
          onKeyDown={e => {
            if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
              e.preventDefault()
              save()
            } else if (e.key === 'Tab') {
              e.preventDefault()
              const el = e.currentTarget
              el.setRangeText('  ', el.selectionStart, el.selectionEnd, 'end')
              setText(el.value)
              textRef.current = el.value
              setDirty(true)
            }
          }}
        />
      </div>
      {ask === 'changed' && (
        <Modal title={t('editor.changedTitle')} width={460} onClose={() => answer.current(false)}>
          <p className="mt-0">{t('editor.changed', { path })}</p>
          <div className="mt-[18px] flex justify-end gap-2">
            <button className={smallBtn} onClick={() => answer.current(false)}>{t('editor.cancel')}</button>
            <button className="btn-primary h-7 rounded-lg px-2.5 text-[12px]" onClick={() => answer.current(true)}>{t('editor.overwrite')}</button>
          </div>
        </Modal>
      )}
      {ask === 'unsaved' && (
        <Modal title={t('editor.unsavedTitle')} width={460} onClose={() => answer.current(false)}>
          <p className="mt-0">{t('editor.unsaved', { name: baseName(path) })}</p>
          <div className="mt-[18px] flex items-center gap-2">
            <button className={smallBtn} onClick={() => answer.current(true)}>{t('editor.discard')}</button>
            <div className="flex-1" />
            <button className={smallBtn} onClick={() => answer.current(false)}>{t('editor.cancel')}</button>
            <button className="btn-primary h-7 rounded-lg px-2.5 text-[12px]" onClick={() => answer.current('save')}>{t('editor.saveClose')}</button>
          </div>
        </Modal>
      )}
    </>
  )
}

const LOG_CHUNK = 64 * 1024
const LOG_MAX_LINES = 20000

function LogLines({ lines, filter }: { lines: string[]; filter: string }) {
  const f = filter.toLowerCase()
  return lines.filter(l => !f || l.toLowerCase().includes(f)).map((l, i) => {
    const p = logParts(l)
    return (
      <div key={i} className="break-all whitespace-pre-wrap">
        <span className="text-[#5c6779]">{p.head}</span>
        <span className={p.c === 'warn' ? 'text-warn' : p.c === 'err' ? 'text-err' : p.c === 'ok' ? 'text-ok' : ''}>{p.body}</span>
      </div>
    )
  })
}

// ログの追従。末尾から読み、追記を足していく。上の端までスクロールしたら前を読み足す
export function LogView({ win, serverId, home }: { win: Win; serverId: string; home: string }) {
  const { t } = useTranslation()
  const settings = useSettings()
  const path = win.path!
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState('')
  const [filter, setFilter] = useState('')
  const [follow, setFollow] = useState(settings.followLog)
  const out = useRef<HTMLDivElement>(null)
  const start = useRef(-1) // 読んだ範囲の頭のバイトの位置。0 ならファイルの頭まで読んだ
  const rest = useRef('') // 改行で終わっていない最後の行
  const loading = useRef(false)
  const keepBottom = useRef<number | null>(null)

  useEffect(() => {
    let id = ''
    let detach = () => {}
    let disposed = false
    LogTail(serverId, path, -1, LOG_CHUNK).then(async c => {
      if (disposed) return
      start.current = c.start
      const parts = c.text.split('\n')
      rest.current = parts.pop() ?? ''
      setLines(parts)
      id = await LogFollow(serverId, path, c.end)
      if (disposed) { LogStop(id); return }
      detach = attachLog(id, {
        out: text => {
          const parts = (rest.current + text).split('\n')
          rest.current = parts.pop() ?? ''
          if (!parts.length) return
          setLines(l => {
            const next = [...l, ...parts]
            if (next.length <= LOG_MAX_LINES) return next
            // 古い行を捨てたら、前を読み足す位置が分からなくなる
            start.current = -1
            return next.slice(next.length - LOG_MAX_LINES)
          })
        },
        end: e => { if (e) setError(t('log.ended', { error: e })) },
      })
    }, e => setError(errText(e)))
    return () => {
      disposed = true
      detach()
      if (id) LogStop(id)
    }
  }, [serverId, path, t])

  // 追従している間は、行が増えたら末尾を見せる。前を読み足したら、見ていた行の位置を保つ
  useLayoutEffect(() => {
    const el = out.current
    if (!el) return
    if (keepBottom.current !== null) {
      el.scrollTop = el.scrollHeight - keepBottom.current
      keepBottom.current = null
    } else if (follow) el.scrollTop = el.scrollHeight
  }, [lines, follow])

  const onScroll = () => {
    const el = out.current!
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 8
    if (atBottom !== follow) setFollow(atBottom)
    if (el.scrollTop < 40 && start.current > 0 && !loading.current) {
      loading.current = true
      LogTail(serverId, path, start.current, LOG_CHUNK).then(c => {
        start.current = c.start
        const parts = c.text.split('\n')
        if (parts.at(-1) === '') parts.pop()
        keepBottom.current = el.scrollHeight - el.scrollTop
        setLines(l => [...parts, ...l])
      }).finally(() => { loading.current = false })
    }
  }

  return (
    <>
      <div className={toolbar}>
        <span className={pathCls}>{path.startsWith(home + '/') ? '~' + path.slice(home.length) : path}</span>
        <div className="flex-1" />
        <input className="field h-6 w-[130px] rounded-md border border-line-strong bg-field px-2 text-[12px] text-text outline-none" placeholder={t('log.filter')} spellCheck={false} value={filter} onChange={e => setFilter(e.target.value)} />
        <label className="inline-flex cursor-pointer items-center gap-1.5 text-muted select-none">
          <input type="checkbox" className="accent-[var(--accent)]" checked={follow} onChange={e => setFollow(e.target.checked)} />{t('log.follow')}
        </label>
      </div>
      <div ref={out} data-log className="min-h-0 flex-1 overflow-auto bg-term px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-[#cfd6e0]" onScroll={onScroll}>
        <LogLines lines={lines} filter={filter} />
        {error && <div className="text-err">{error}</div>}
      </div>
    </>
  )
}

const MIME: Record<string, string> = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml', bmp: 'image/bmp', ico: 'image/x-icon' }

export function ImageView({ win, serverId }: { win: Win; serverId: string }) {
  const path = win.path!
  const [src, setSrc] = useState('')
  const [size, setSize] = useState(0)
  const [dim, setDim] = useState('')
  const [error, setError] = useState('')
  useEffect(() => {
    ReadImage(serverId, path).then(b64 => {
      setSize(Math.floor(b64.length * 3 / 4))
      setSrc(`data:${MIME[path.split('.').pop()!.toLowerCase()] ?? 'application/octet-stream'};base64,${b64}`)
    }, e => setError(errText(e)))
  }, [serverId, path])
  if (error) return <Failed text={error} />
  return (
    <>
      <div className="grid min-h-0 flex-1 place-items-center overflow-auto bg-[repeating-conic-gradient(var(--panel-2)_0%_25%,var(--panel)_0%_50%)] bg-[length:20px_20px]">
        {src && <img src={src} alt="" className="max-h-full max-w-full object-contain [image-rendering:pixelated]" onLoad={e => setDim(`${e.currentTarget.naturalWidth} × ${e.currentTarget.naturalHeight}`)} />}
      </div>
      <div className="flex-none border-t border-line px-3 py-1.5 text-[12px] text-muted">{dim}{dim && ' · '}{fmtSize(size)}</div>
    </>
  )
}

type TreeNode = { name: string; path: string; dir: boolean; size: number; children: Map<string, TreeNode> }

// 圧縮ファイルの中身。PC の一時フォルダに落として読み、サーバーには展開しない
export function ArchiveView({ win, serverId, home }: { win: Win; serverId: string; home: string }) {
  const { t } = useTranslation()
  const path = win.path!
  const [info, setInfo] = useState<{ id: string; entries: archive.Entry[] } | null>(null)
  const [error, setError] = useState('')
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const [view, setView] = useState<{ path: string; text: string } | null>(null)
  const [scroll, setScroll] = useState(0)

  useEffect(() => {
    let id = ''
    let disposed = false
    ArchiveOpen(serverId, path).then(i => {
      id = i.id
      if (disposed) { ArchiveClose(id); return }
      setInfo({ id: i.id, entries: i.entries ?? [] })
      // .gz の1つのファイルは、一覧を出さずに中身を見せる
      if (i.entries?.length === 1 && i.entries[0].size === -1) show(i.id, i.entries[0].path)
    }, e => setError(errText(e)))
    return () => {
      disposed = true
      if (id) ArchiveClose(id)
    }
  }, [serverId, path])

  const show = (id: string, entry: string) => ArchiveRead(id, entry).then(text => setView({ path: entry, text }), e => setError(errText(e)))

  const tree = useMemo(() => {
    const root: TreeNode = { name: '', path: '', dir: true, size: 0, children: new Map() }
    for (const e of info?.entries ?? []) {
      const parts = e.path.split('/')
      let n = root
      parts.forEach((seg, i) => {
        const last = i === parts.length - 1
        if (!n.children.has(seg)) n.children.set(seg, { name: seg, path: parts.slice(0, i + 1).join('/'), dir: !last || e.dir, size: last ? e.size : 0, children: new Map() })
        n = n.children.get(seg)!
      })
    }
    return root
  }, [info])

  if (error && !view) return <Failed text={error} />
  if (!info) return <div className="flex-1 bg-term" />
  const shown = path.startsWith(home + '/') ? '~' + path.slice(home.length) : path
  const single = info.entries.length === 1 && info.entries[0].size === -1

  if (view) {
    const lines = view.text.split('\n').length
    return (
      <>
        <div className={toolbar}>
          {!single && <button className={smallBtn} onClick={() => setView(null)}>{t('archive.back')}</button>}
          <span className={pathCls}>{single ? shown : view.path}</span>
          <div className="flex-1" />
          <span className="text-[11.5px] text-muted">{single ? t('archive.extracted') : t('archive.readonly')}</span>
        </div>
        {view.path.toLowerCase().endsWith('.log')
          ? <div className="min-h-0 flex-1 overflow-auto bg-term px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-[#cfd6e0]"><LogLines lines={view.text.split('\n')} filter="" /></div>
          : (
            <div className="flex min-h-0 flex-1 bg-term">
              <Gutter lines={lines} scrollTop={scroll} />
              <pre className="m-0 flex-1 overflow-auto px-3 py-2 font-mono text-[12.5px] leading-[1.5] text-[#eef2f8]" onScroll={e => setScroll(e.currentTarget.scrollTop)}>{view.text}</pre>
            </div>
          )}
      </>
    )
  }

  const total = info.entries.reduce((a, e) => a + Math.max(0, e.size), 0)
  const rows: React.ReactNode[] = []
  const walk = (n: TreeNode, d: number) => {
    const kids = [...n.children.values()].sort((a, b) => (+b.dir - +a.dir) || a.name.localeCompare(b.name))
    for (const c of kids) {
      const pad = { paddingLeft: `calc(12px + ${d} * 16px)` }
      if (c.dir) {
        const isOpen = open[c.path] ?? d === 0
        rows.push(
          <div key={c.path} style={pad} className="flex h-[26px] cursor-pointer items-center gap-1.5 pr-3 text-[12.5px] select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)]" onClick={() => setOpen(o => ({ ...o, [c.path]: !isOpen }))}>
            <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
            <Icon name="folder" size={15} className="f-dir" />
            <span className="min-w-0 flex-1 truncate">{c.name}</span>
          </div>,
        )
        if (isOpen) walk(c, d + 1)
        continue
      }
      const k = fileKind(c.name, false)
      const [icon, color] = KIND_ICON[k]
      const readable = k !== 'bin' && k !== 'img' && k !== 'arc'
      rows.push(
        <div key={c.path} style={pad} data-entry={c.path} className={`flex h-[26px] items-center gap-1.5 pr-3 text-[12.5px] select-none ${readable ? 'cursor-pointer hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)]' : 'cursor-default text-muted'}`} onClick={readable ? () => show(info.id, c.path) : undefined}>
          <span className="w-[13px] flex-none" />
          <Icon name={icon} size={15} className={color} />
          <span className="min-w-0 flex-1 truncate">{c.name}</span>
          <span className="font-mono text-[11.5px] text-faint">{fmtSize(c.size)}</span>
        </div>,
      )
    }
  }
  walk(tree, 0)
  return (
    <>
      <div className={toolbar}>
        <span className={pathCls}>{shown}</span>
        <div className="flex-1" />
        <span className="text-[11.5px] text-muted">{t('archive.count', { n: info.entries.filter(e => !e.dir).length, size: fmtSize(total) })}</span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto bg-[var(--win-bg)] py-1">{rows}</div>
    </>
  )
}
