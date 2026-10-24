import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListDir, NewEntry, PasteEntry, RenameEntry, RestoreEntry, TrashEntry } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { pathArg, terminalOf } from '../terminals'
import { toast } from '../toasts'
import { Icon, type IconName } from './Icon'
import { Menu, type MenuItem } from './Menu'

type Entry = remote.Entry
type Dir = { entries?: Entry[]; error?: string; loading: boolean }
// 右クリックのメニューと名前の変更は、ツリーの1行を「パスとフォルダかどうか」で扱う
type Node = { path: string; name: string; dir: boolean; root: boolean }

// 種類ごとのアイコンと色。mock/app.js の fileKind と同じ分け方
function kindOf(e: { name: string; dir: boolean }): [IconName, string] {
  if (e.dir) return ['folder', 'f-dir']
  const n = e.name.toLowerCase()
  if (n.endsWith('.log')) return ['log', 'f-log']
  if (/\.(png|jpe?g|gif|webp)$/.test(n)) return ['image', 'f-img']
  if (/\.(gz|tgz|tar|zip|jar|7z)$/.test(n)) return ['archive', 'f-arc']
  if (/\.(sh|json|ya?ml|toml|properties|js|ts|py|env|conf|ini)$/.test(n)) return ['code', 'f-code']
  return ['file', 'f-file']
}

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
  const box = useRef<HTMLDivElement>(null)

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
  const fail = (e: unknown) => toast(errText(e), 'warn')

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

  // エクスプローラーと同じ並び。開く系 → 新規 → 切り取り・コピー・貼り付け → 名前・削除 → パス
  const menuItems = (n: Node): (MenuItem | false)[] => [
    n.dir
      ? { icon: 'terminal', label: t('menu.cd'), fn: () => terminalOf(serverId)?.cd(pathArg(root, n.path)) }
      : { icon: 'terminal', label: t('menu.insertPath'), fn: () => terminalOf(serverId)?.insert(pathArg(root, n.path)) },
    { sep: true },
    n.dir && { icon: 'filePlus', label: t('menu.newFile'), fn: () => create(n.path, false) },
    n.dir && { icon: 'folderPlus', label: t('menu.newFolder'), fn: () => create(n.path, true) },
    { sep: true },
    { icon: 'cut', label: t('menu.cut'), hint: 'Ctrl+X', disabled: n.root, fn: () => setClip({ path: n.path, move: true }) },
    { icon: 'copy', label: t('menu.copy'), hint: 'Ctrl+C', disabled: n.root, fn: () => setClip({ path: n.path, move: false }) },
    { icon: 'paste', label: t('menu.paste'), hint: 'Ctrl+V', disabled: !clip, fn: () => paste(n.dir ? n.path : parentOf(n.path)) },
    { sep: true },
    { icon: 'edit', label: t('menu.rename'), hint: 'F2', disabled: n.root, fn: () => setRenaming(n.path) },
    { icon: 'trash', label: t('menu.delete'), hint: 'Delete', disabled: n.root, danger: true, fn: () => trash(n) },
    { sep: true },
    { icon: 'link', label: t('menu.copyPath'), fn: () => copyPath(n.path) },
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

  const rows = (path: string, depth: number): React.ReactNode[] => {
    const d = dirs[path]
    if (!d || (d.loading && !d.entries)) return []
    if (d.error) return [<Note key={path + ':err'} depth={depth} text={t('files.error', { error: d.error })} err />]
    if (d.entries!.length === 0) return [<Note key={path + ':empty'} depth={depth} text={t('files.empty')} />]
    return d.entries!.flatMap(e => {
      const [icon, color] = kindOf(e)
      const isOpen = !!open[e.path]
      const cut = clip?.move && clip.path === e.path
      return [
        <div
          key={e.path}
          className={`tree-row relative flex h-6 cursor-pointer items-center gap-[5px] pr-1 text-[13px] whitespace-nowrap select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${selected === e.path ? 'bg-accent-soft shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--accent)_35%,transparent)]' : ''} ${cut ? 'opacity-50' : ''}`}
          style={{ paddingLeft: `calc(10px + ${depth} * 14px)`, ['--d' as string]: depth }}
          data-d={depth}
          data-path={e.path}
          onClick={() => click(e)}
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
    })
  }

  return (
    <div ref={box} tabIndex={0} className="min-h-0 flex-1 overflow-auto pb-5 outline-none" onKeyDown={onKey}>
      <div
        className={`mb-0.5 flex h-[26px] items-center gap-1.5 px-2.5 font-mono text-[11.5px] text-muted ${selected === root ? 'bg-accent-soft' : ''}`}
        title={root}
        onClick={() => setSelected(root)}
        onContextMenu={ev => contextMenu(ev, nodeOf(root))}
      >
        <Icon name="folder" size={14} className="f-dir" />
        <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{root}</span>
      </div>
      {rows(root, 0)}
      {menu && <Menu x={menu.x} y={menu.y} items={menuItems(menu.node)} onClose={() => setMenu(null)} />}
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
