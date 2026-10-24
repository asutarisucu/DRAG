import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, ListDir, LocalEntries, NewEntry, PasteEntry, PickUploadFiles, RenameEntry, RestoreEntry, Stat, TrashEntry, Upload } from '../../wailsjs/go/main/App'
import type { main, remote } from '../../wailsjs/go/models'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { pathArg, terminalOf } from '../terminals'
import { useSettings } from '../settings'
import { progressToast, toast } from '../toasts'
import { withProgress } from '../transfers'
import { ConflictDialog, PropsDialog, type ConflictAnswer } from './FileDialogs'
import { Icon } from './Icon'
import { Menu, type MenuItem } from './Menu'
import { KIND_ICON, fileKind } from '../fileKind'
import { openFile } from '../openFile'

type Entry = remote.Entry
type Dir = { entries?: Entry[]; error?: string; loading: boolean }
// 右クリックのメニューと名前の変更は、ツリーの1行を「パスとフォルダかどうか」で扱う
type Node = { path: string; name: string; dir: boolean; root: boolean }

const parentOf = (p: string) => p.slice(0, p.lastIndexOf('/')) || '/'
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const errText = (e: unknown) => String(e).replace(/^Error: /, '')

// 左のファイルツリー。フォルダは開いたときに中を読む。
// generation は再接続のたびに増える。増えたら開いているフォルダを読み直す
export function FileTree({ serverId, root, generation }: { serverId: string; root: string; generation: number }) {
  const { t } = useTranslation()
  const [dirs, setDirs] = useState<Record<string, Dir>>({})
  const [open, setOpen] = useState<Record<string, boolean>>({ [root]: true })
  const [selected, setSelected] = useState('')
  const [renaming, setRenaming] = useState('')
  const [clip, setClip] = useState<{ path: string; move: boolean } | null>(null)
  const [menu, setMenu] = useState<{ x: number; y: number; node: Node } | null>(null)
  const [uploads, setUploads] = useState<{ key: string; dir: string; name: string; isDir: boolean; p: number }[]>([])
  const [conflict, setConflict] = useState<{ path: string; more: boolean; resolve: (a: ConflictAnswer) => void } | null>(null)
  const [props, setProps] = useState<{ node: Node; props: remote.Props } | null>(null)
  const [dropDir, setDropDir] = useState('')
  const box = useRef<HTMLDivElement>(null)
  const settings = useSettings()

  const load = useCallback((path: string) => {
    setDirs(d => ({ ...d, [path]: { ...d[path], loading: true } }))
    return ListDir(serverId, path).then(
      entries => setDirs(d => ({ ...d, [path]: { entries: entries ?? [], loading: false } })),
      err => setDirs(d => ({ ...d, [path]: { error: errText(err), loading: false } })),
    )
  }, [serverId])

  useEffect(() => { load(root) }, [load, root])

  useEffect(() => {
    if (generation === 0) return
    // 開いているフォルダは、再接続した時点のものだけを読み直せばよい
    for (const [path, isOpen] of Object.entries(open)) if (isOpen) load(path)
  }, [generation])

  const nodeOf = (path: string): Node => {
    if (path === root) return { path, name: baseOf(path), dir: true, root: true }
    const e = dirs[parentOf(path)]?.entries?.find(x => x.path === path)
    return { path, name: baseOf(path), dir: !!e?.dir, root: false }
  }

  const focusTree = () => box.current?.focus({ preventScroll: true })

  // 操作のあとに、変わったフォルダを開いて読み直し、できたものを選ぶ
  const after = async (changed: string[], select?: string) => {
    for (const d of new Set(changed)) {
      setOpen(o => ({ ...o, [d]: true }))
      await load(d)
    }
    if (select) setSelected(select)
  }
  // SFTP の英語の決まり文句は、画面の言語の文に置き換える
  const fail = (e: unknown) => toast(errText(e) === 'permission denied' ? t('files.denied') : errText(e), 'warn')

  const create = (dir: string, isDir: boolean) => {
    NewEntry(serverId, dir, isDir ? t('files.newFolderName') : t('files.newFileName'), isDir).then(async p => {
      await after([dir], p)
      setRenaming(p)
    }, fail)
  }
  const rename = (node: Node, name: string) => {
    setRenaming('')
    focusTree()
    if (!name || name === node.name) return
    RenameEntry(serverId, node.path, name).then(p => after([parentOf(node.path)], p), fail)
  }
  const paste = (dir: string) => {
    if (!clip) return
    PasteEntry(serverId, clip.path, dir, clip.move).then(p => {
      if (clip.move) {
        setClip(null)
        toast(t('toast.moved', { name: baseOf(p), dir: pathArg(root, dir) }), 'ok')
      }
      after(clip.move ? [parentOf(clip.path), dir] : [dir], p)
    }, fail)
  }
  const trash = (node: Node) => {
    TrashEntry(serverId, node.path).then(item => {
      after([parentOf(node.path)], parentOf(node.path))
      toast(t('toast.trashed', { name: node.name }), 'info', {
        label: t('toast.undo'),
        fn: () => RestoreEntry(serverId, item).then(p => {
          after([parentOf(p)], p)
          toast(t('toast.restored', { name: baseOf(p) }), 'ok')
        }, fail),
      })
    }, fail)
  }
  const copyPath = (p: string) => ClipboardSetText(p).then(() => toast(t('toast.pathCopied'), 'ok'))

  const ask = (path: string, more: boolean) => new Promise<ConflictAnswer>(resolve => setConflict({ path, more, resolve }))

  // PC のファイルを dir へ送る。同じ名前があれば設定に従い、「毎回聞く」なら1件ずつ聞く
  const uploadTo = async (dir: string, paths: string[]) => {
    if (!paths.length) return
    let items: main.LocalEntry[]
    try {
      items = await LocalEntries(paths)
    } catch (e) {
      fail(e)
      return
    }
    const existing = new Set((await ListDir(serverId, dir).catch(() => [])).map(e => e.name))
    let fixed: ConflictAnswer | null = settings.uploadConflict === 'ask' ? null : { choice: settings.uploadConflict, all: true }
    let count = 0, last = ''
    for (let i = 0; i < items.length; i++) {
      const it = items[i]
      let keep = false
      if (existing.has(it.name)) {
        const ans = fixed ?? await ask(pathArg(root, dir + '/' + it.name), i < items.length - 1)
        setConflict(null)
        if (ans.all) fixed = ans
        if (ans.choice === 'skip') continue
        keep = ans.choice === 'keep'
      }
      setOpen(o => ({ ...o, [dir]: true }))
      try {
        last = await withProgress(
          (done, total) => setUploads(u => u.map(x => x.key === it.path ? { ...x, p: total ? done / total : 1 } : x)),
          key => {
            setUploads(u => [...u, { key: it.path, dir, name: it.name, isDir: it.dir, p: 0 }])
            return Upload(serverId, it.path, dir, keep, key)
          })
        count++
      } catch (e) {
        fail(e)
      } finally {
        setUploads(u => u.filter(x => x.key !== it.path))
      }
    }
    await after([dir], last || undefined)
    if (count) toast(t('upload.sent', { count, dir: pathArg(root, dir) }), 'ok')
  }
  const pickUpload = (dir: string) => PickUploadFiles().then(paths => uploadTo(dir, paths ?? []), fail)

  const download = (n: Node) => {
    const pt = progressToast(t('upload.receiving', { name: n.name }))
    withProgress((done, total) => pt.set(total ? done / total : 1), key => Download(serverId, n.path, key)).then(
      local => local ? pt.done(t('upload.received', { path: local })) : pt.done(t('connect.cancel'), 'info'),
      e => pt.done(errText(e), 'warn'),
    )
  }
  const showProps = (n: Node) => Stat(serverId, n.path).then(pr => setProps({ node: n, props: pr }), fail)

  // Wails が知らせる PC からのドロップ。どの行に落ちたかは App が調べて渡す
  useEffect(() => {
    const onDrop = (e: Event) => {
      const d = (e as CustomEvent<{ serverId: string; dir: string; paths: string[] }>).detail
      if (d.serverId === serverId) uploadTo(d.dir, d.paths)
    }
    window.addEventListener('drag:file-drop', onDrop)
    return () => window.removeEventListener('drag:file-drop', onDrop)
  })

  // エクスプローラーと同じ並び。開く系 → 新規 → 切り取り・コピー・貼り付け → 名前・削除 → パス
  const menuItems = (n: Node): (MenuItem | false)[] => [
    !n.dir && fileKind(n.name, false) === 'arc' && { icon: 'archive', label: t('open.extract'), fn: () => openFile(serverId, n) },
    !n.dir && !['arc', 'bin'].includes(fileKind(n.name, false)) && { icon: 'window', label: t('open.onDesktop'), fn: () => openFile(serverId, n) },
    n.dir
      ? { icon: 'terminal', label: t('menu.cd'), fn: () => terminalOf(serverId)?.cd(pathArg(root, n.path)) }
      : { icon: 'terminal', label: t('menu.insertPath'), fn: () => terminalOf(serverId)?.insert(pathArg(root, n.path)) },
    { sep: true },
    n.dir && { icon: 'filePlus', label: t('menu.newFile'), fn: () => create(n.path, false) },
    n.dir && { icon: 'folderPlus', label: t('menu.newFolder'), fn: () => create(n.path, true) },
    n.dir && { icon: 'upload', label: t('menu.uploadHere'), fn: () => pickUpload(n.path) },
    { sep: true },
    { icon: 'cut', label: t('menu.cut'), hint: 'Ctrl+X', disabled: n.root, fn: () => setClip({ path: n.path, move: true }) },
    { icon: 'copy', label: t('menu.copy'), hint: 'Ctrl+C', disabled: n.root, fn: () => setClip({ path: n.path, move: false }) },
    { icon: 'paste', label: t('menu.paste'), hint: 'Ctrl+V', disabled: !clip, fn: () => paste(n.dir ? n.path : parentOf(n.path)) },
    { sep: true },
    { icon: 'edit', label: t('menu.rename'), hint: 'F2', disabled: n.root, fn: () => setRenaming(n.path) },
    { icon: 'trash', label: t('menu.delete'), hint: 'Delete', disabled: n.root, danger: true, fn: () => trash(n) },
    { sep: true },
    { icon: 'link', label: t('menu.copyPath'), fn: () => copyPath(n.path) },
    { icon: 'download', label: n.dir ? t('menu.downloadZip') : t('menu.download'), disabled: n.path === '/', fn: () => download(n) },
    { icon: 'info', label: t('menu.props'), fn: () => showProps(n) },
  ]

  const onKey = (e: React.KeyboardEvent) => {
    if (!selected || renaming) return
    const n = nodeOf(selected)
    const ctrl = e.ctrlKey || e.metaKey
    if (e.key === 'F2' && !n.root) setRenaming(n.path)
    else if (e.key === 'Delete' && !n.root) trash(n)
    else if (ctrl && e.key === 'c' && !n.root) setClip({ path: n.path, move: false })
    else if (ctrl && e.key === 'x' && !n.root) setClip({ path: n.path, move: true })
    else if (ctrl && e.key === 'v') paste(n.dir ? n.path : parentOf(n.path))
    else return
    e.preventDefault()
  }

  const click = (e: Entry) => {
    setSelected(e.path)
    if (!e.dir) return
    const next = !open[e.path]
    setOpen(o => ({ ...o, [e.path]: next }))
    if (next && !dirs[e.path]?.entries) load(e.path)
  }
  const contextMenu = (ev: React.MouseEvent, n: Node) => {
    ev.preventDefault()
    ev.stopPropagation()
    setSelected(n.path)
    setMenu({ x: ev.clientX, y: ev.clientY, node: n })
  }

  // PC からのドラッグ中は、落とす先のフォルダの行を緑で囲む。ファイルの行ならそのファイルのあるフォルダ
  const dropProps = (dir: string) => ({
    'data-drop-dir': dir,
    onDragOver: (ev: React.DragEvent) => { if (ev.dataTransfer.types.includes('Files')) setDropDir(dir) },
    onDragLeave: () => setDropDir(d => d === dir ? '' : d),
    onDrop: () => setDropDir(''),
  })
  const dropOn = 'bg-[rgba(63,207,142,.14)] shadow-[inset_0_0_0_1px_var(--ok)]'

  const pending = (path: string, depth: number) => uploads.filter(u => u.dir === path).map(u => (
    <div key={'up:' + u.key} className="relative flex h-6 items-center gap-[5px] pr-1 text-[13px] whitespace-nowrap text-muted" style={{ paddingLeft: `calc(10px + ${depth} * 14px)` }}>
      <span className="w-[13px] flex-none" />
      <span className={u.isDir ? 'f-dir' : 'f-file'}><Icon name={u.isDir ? 'folder' : 'file'} size={15} /></span>
      <span className="min-w-0 flex-1 overflow-hidden text-ellipsis">{u.name}</span>
      <div className="h-1 w-16 flex-none overflow-hidden rounded-sm bg-line"><i className="block h-full bg-gradient-to-r from-ok to-[#7fe6b8]" style={{ width: `${u.p * 100}%` }} /></div>
      <span className="pl-2 font-mono text-[11px] text-faint">{Math.round(u.p * 100)}%</span>
    </div>
  ))

  const rows = (path: string, depth: number): React.ReactNode[] => {
    const d = dirs[path]
    if (!d || (d.loading && !d.entries)) return pending(path, depth)
    if (d.error) return [<Note key={path + ':err'} depth={depth} text={t('files.error', { error: d.error })} err />]
    if (d.entries!.length === 0 && !uploads.some(u => u.dir === path)) return [<Note key={path + ':empty'} depth={depth} text={t('files.empty')} />]
    return [...d.entries!.flatMap(e => {
      const [icon, color] = KIND_ICON[fileKind(e.name, e.dir)]
      const isOpen = !!open[e.path]
      const cut = clip?.move && clip.path === e.path
      return [
        <div
          key={e.path}
          className={`tree-row relative flex h-6 cursor-pointer items-center gap-[5px] pr-1 text-[13px] whitespace-nowrap select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${dropDir && dropDir === (e.dir ? e.path : path) ? dropOn : selected === e.path ? 'bg-accent-soft shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--accent)_35%,transparent)]' : ''} ${cut ? 'opacity-50' : ''}`}
          style={{ paddingLeft: `calc(10px + ${depth} * 14px)`, ['--d' as string]: depth }}
          data-d={depth}
          data-path={e.path}
          {...dropProps(e.dir ? e.path : path)}
          onClick={() => click(e)}
          onDoubleClick={() => { if (!e.dir) openFile(serverId, e) }}
          draggable={renaming !== e.path}
          onDragStart={ev => {
            ev.dataTransfer.setData('application/x-drag-path', JSON.stringify({ path: e.path, name: e.name, dir: e.dir }))
            if (e.dir) ev.dataTransfer.setData('application/x-drag-dir', '1')
            ev.dataTransfer.effectAllowed = 'copyMove'
          }}
          onContextMenu={ev => contextMenu(ev, { path: e.path, name: e.name, dir: e.dir, root: false })}
          title={e.path}
        >
          {e.dir
            ? <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
            : <span className="w-[13px] flex-none" />}
          <span className={color}><Icon name={icon} size={15} /></span>
          {renaming === e.path
            ? <RenameInput name={e.name} dir={e.dir} onDone={name => rename({ path: e.path, name: e.name, dir: e.dir, root: false }, name)} />
            : <span className="min-w-0 flex-1 overflow-hidden text-ellipsis">{e.name}</span>}
          {e.link && <span className="text-faint"><Icon name="link" size={12} /></span>}
        </div>,
        ...(e.dir && isOpen ? rows(e.path, depth + 1) : []),
      ]
    }), ...pending(path, depth)]
  }

  return (
    <div ref={box} tabIndex={0} className="min-h-0 flex-1 overflow-auto pb-5 outline-none" onKeyDown={onKey}>
      <div
        className={`mb-0.5 flex h-[26px] items-center gap-1.5 px-2.5 font-mono text-[11.5px] text-muted ${dropDir === root ? dropOn : selected === root ? 'bg-accent-soft' : ''}`}
        title={root + '\n' + t('upload.dropHint')}
        {...dropProps(root)}
        onClick={() => setSelected(root)}
        onContextMenu={ev => contextMenu(ev, nodeOf(root))}
      >
        <Icon name="folder" size={14} className="f-dir" />
        <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{root}</span>
      </div>
      {rows(root, 0)}
      {menu && <Menu x={menu.x} y={menu.y} items={menuItems(menu.node)} onClose={() => setMenu(null)} />}
      {conflict && <ConflictDialog path={conflict.path} more={conflict.more} onAnswer={conflict.resolve} />}
      {props && <PropsDialog name={props.node.name} path={props.node.path} kind={fileKind(props.node.name, props.props.mode.startsWith('d'))} props={props.props} onClose={() => setProps(null)} />}
    </div>
  )
}

// 名前の変更の入力欄。拡張子の前までを選んだ状態で始める
function RenameInput({ name, dir, onDone }: { name: string; dir: boolean; onDone: (name: string) => void }) {
  const ref = useRef<HTMLInputElement>(null)
  const done = useRef(false)
  useEffect(() => {
    const i = ref.current!
    i.focus()
    const dot = dir ? -1 : name.lastIndexOf('.')
    i.setSelectionRange(0, dot > 0 ? dot : name.length)
  }, [name, dir])
  const finish = (v: string) => {
    if (done.current) return
    done.current = true
    onDone(v.trim())
  }
  return (
    <input
      ref={ref}
      defaultValue={name}
      spellCheck={false}
      className="h-5 min-w-0 flex-1 rounded border border-accent bg-field px-1 text-[13px] text-text outline-none"
      onClick={e => e.stopPropagation()}
      onKeyDown={e => {
        e.stopPropagation()
        if (e.key === 'Enter') finish(e.currentTarget.value)
        if (e.key === 'Escape') finish(name)
      }}
      onBlur={e => finish(e.currentTarget.value)}
    />
  )
}

function Note({ depth, text, err = false }: { depth: number; text: string; err?: boolean }) {
  return (
    <div className={`h-6 truncate text-[12px] leading-6 ${err ? 'text-err' : 'text-faint'}`} style={{ paddingLeft: `calc(28px + ${depth} * 14px)` }}>
      {text}
    </div>
  )
}
