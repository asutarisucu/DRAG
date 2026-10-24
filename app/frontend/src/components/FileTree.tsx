import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, ListDir, LocalEntries, NewEntry, PasteEntry, PickUploadFiles, RenameEntry, RestoreEntry, Stat, TrashEntry, Upload } from '../../wailsjs/go/main/App'
import type { main, remote } from '../../wailsjs/go/models'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { pathArg, terminalOf } from '../terminals'
import { useSettings } from '../settings'
import { setMTab, useMobile } from '../mobile'
import { isRemote } from '../remote'
import i18n from '../i18n'
import { isDenied, useSudo, withSudo } from '../sudo'
import { fmtSize, progressToast, toast } from '../toasts'
import { withProgress } from '../transfers'
import { ConflictDialog, PropsDialog, type ConflictAnswer } from './FileDialogs'
import { Icon } from './Icon'
import { Menu, type MenuItem } from './Menu'
import { KIND_ICON, fileKind } from '../fileKind'
import { openFile } from '../openFile'
import { openTool } from '../tools'
import { updateWin, winsOf } from '../windows'

type Entry = remote.Entry
type Dir = { entries?: Entry[]; error?: string; loading: boolean }
// 右クリックのメニューと名前の変更は、ツリーの1行を「パスとフォルダかどうか」で扱う。
// top はツリーのいちばん上やホームのように、移したり消したりさせない行
type Node = { path: string; name: string; dir: boolean; top: boolean }

const parentOf = (p: string) => p.slice(0, p.lastIndexOf('/')) || '/'
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const errText = (e: unknown) => String(e).replace(/^Error: /, '')

// 左のファイルツリー。フォルダは開いたときに中を読む。root はホーム。
// generation は再接続のたびに増える。増えたら開いているフォルダを読み直す。
// ホームの外へ上がるのと、root のものを変えるのは、sudo モードのときだけにする
export function FileTree({ serverId, root, generation }: { serverId: string; root: string; generation: number }) {
  const { t } = useTranslation()
  const [top, setTop] = useState(root)
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
  // 貼り付け・移動・アップロード・元に戻す・ツリーで表示のあとに、その行を少しのあいだ光らせる
  const [flash, setFlash] = useState('')
  const flashTimer = useRef(0)
  const flashRow = (p: string) => {
    setFlash(p)
    clearTimeout(flashTimer.current)
    flashTimer.current = window.setTimeout(() => setFlash(''), 1200)
  }
  const box = useRef<HTMLDivElement>(null)
  const settings = useSettings()
  const sudo = useSudo(serverId)
  // スマホではドラッグの代わりに、行の … と長押しでメニューを出す
  const mobile = useMobile()
  const longPressed = useRef(false)
  // ツリーの中でドラッグしている行のパス。フォルダへ落とすと移す
  const dragging = useRef('')
  // PC のファイルをアプリの上へドラッグしている間は、ツリーを緑で囲み、下に落とす先を出す
  const [osDrag, setOsDrag] = useState(false)
  useEffect(() => {
    let depth = 0
    const isFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files')
    const end = () => {
      depth = 0
      setOsDrag(false)
      setDropDir('')
    }
    const enter = (e: DragEvent) => { if (isFiles(e) && depth++ === 0) setOsDrag(true) }
    const leave = (e: DragEvent) => { if (isFiles(e) && --depth <= 0) end() }
    window.addEventListener('dragenter', enter)
    window.addEventListener('dragleave', leave)
    window.addEventListener('drop', end)
    return () => {
      window.removeEventListener('dragenter', enter)
      window.removeEventListener('dragleave', leave)
      window.removeEventListener('drop', end)
    }
  }, [])
  // ツリーで表示したあとに、その行が見えるようにスクロールする
  const scrollTo = useRef('')
  useEffect(() => {
    const row = scrollTo.current && box.current?.querySelector(`[data-path="${CSS.escape(scrollTo.current)}"]`)
    if (!row) return
    row.scrollIntoView({ block: 'nearest' })
    scrollTo.current = ''
  })

  // フォルダの中を読む。読めればから文字列、読めなければその理由を返す。
  // ask なら、権限で読めないときに sudo モードを ON にするかを聞く。開いたときにだけ聞き、読み直しでは聞かない
  const load = useCallback((path: string, ask = false): Promise<string> => {
    setDirs(d => ({ ...d, [path]: { ...d[path], loading: true } }))
    const list = () => ListDir(serverId, path)
    return (ask ? withSudo(serverId, i18n.t('sudo.openDir', { dir: path }), list) : list()).then(
      entries => {
        setDirs(d => ({ ...d, [path]: { entries: entries ?? [], loading: false } }))
        return ''
      },
      err => {
        setDirs(d => ({ ...d, [path]: { error: errText(err), loading: false } }))
        return errText(err)
      },
    )
  }, [serverId])

  useEffect(() => { load(top) }, [load, top])

  useEffect(() => {
    if (generation === 0) return
    // 開いているフォルダは、再接続した時点のものだけを読み直せばよい
    for (const [path, isOpen] of Object.entries(open)) if (isOpen) load(path)
  }, [generation])

  // sudo モードを切り替えると見えるものが変わるので、開いているフォルダを読み直す。
  // OFF にして、いちばん上のフォルダを読めなくなったら、ホームに戻す
  const prevSudo = useRef(sudo)
  useEffect(() => {
    if (prevSudo.current === sudo) return
    prevSudo.current = sudo
    load(top).then(err => { if (err && top !== root) setTop(root) })
    for (const [path, isOpen] of Object.entries(open)) if (isOpen && path !== top) load(path)
  }, [sudo])

  // ツリーのいちばん上を p に移す。読めないときは sudo モードを聞き、やめたら移らない
  const goTo = (p: string) => load(p, true).then(err => {
    if (err) {
      if (!isDenied(err)) fail(err)
      return
    }
    setOpen(o => ({ ...o, [p]: true, [top]: true }))
    setTop(p)
    setSelected(p)
  })

  const entryOf = (path: string) => dirs[parentOf(path)]?.entries?.find(x => x.path === path)
  const nodeOf = (path: string): Node => {
    const fixed = path === top || path === root || path === parentOf(root) || path === '/'
    return { path, name: baseOf(path), dir: path === top || !!entryOf(path)?.dir, top: fixed }
  }

  const focusTree = () => box.current?.focus({ preventScroll: true })

  // 操作のあとに、変わったフォルダを開いて読み直し、できたものを選ぶ。shine ならその行を光らせる
  const after = async (changed: string[], select?: string, shine = false) => {
    for (const d of new Set(changed)) {
      setOpen(o => ({ ...o, [d]: true }))
      await load(d)
    }
    if (!select) return
    setSelected(select)
    if (shine) flashRow(select)
  }
  // 名前を変えたり移したりしたら、開いているウィンドウのファイルの場所を合わせる
  const moved = (from: string, to: string) => {
    for (const w of winsOf(serverId)) {
      if (!w.path || (w.path !== from && !w.path.startsWith(from + '/'))) continue
      // エディターは題名を自分で付け直す。ほかのウィンドウの題名はファイルの名前
      updateWin(w.id, { path: to + w.path.slice(from.length), ...(w.path === from && w.kind !== 'editor' ? { title: baseOf(to) } : {}) })
    }
  }
  // SFTP の英語の決まり文句は、画面の言語の文に置き換える
  const fail = (e: unknown) => toast(isDenied(e) ? t('files.denied') : errText(e), 'warn')

  // 操作は、ログインしたユーザーの権限で失敗したときだけ sudo モードを聞いてやり直す
  const create = (dir: string, isDir: boolean) => {
    withSudo(serverId, t('sudo.create', { dir: pathArg(root, dir) }), () => NewEntry(serverId, dir, isDir ? t('files.newFolderName') : t('files.newFileName'), isDir)).then(async p => {
      await after([dir], p)
      setRenaming(p)
    }, fail)
  }
  const startRename = (n: Node) => setRenaming(n.path)
  const rename = (node: Node, name: string) => {
    setRenaming('')
    focusTree()
    if (!name || name === node.name) return
    withSudo(serverId, t('sudo.rename', { name: node.name }), () => RenameEntry(serverId, node.path, name)).then(p => {
      moved(node.path, p)
      after([parentOf(node.path)], p)
    }, fail)
  }
  const paste = (dir: string) => {
    if (!clip) return
    withSudo(serverId, t('sudo.paste'), () => PasteEntry(serverId, clip.path, dir, clip.move)).then(p => {
      if (clip.move) {
        setClip(null)
        moved(clip.path, p)
        toast(t('toast.moved', { name: baseOf(p), dir: pathArg(root, dir) }), 'ok')
      }
      after(clip.move ? [parentOf(clip.path), dir] : [dir], p, true)
    }, fail)
  }
  const move = (from: string, dir: string) => {
    withSudo(serverId, t('sudo.move', { name: baseOf(from) }), () => PasteEntry(serverId, from, dir, true)).then(p => {
      moved(from, p)
      toast(t('toast.moved', { name: baseOf(p), dir: pathArg(root, dir) }), 'ok')
      after([parentOf(from), dir], p, true)
    }, fail)
  }
  const trash = (node: Node) => {
    withSudo(serverId, t('sudo.delete', { name: node.name }), () => TrashEntry(serverId, node.path)).then(item => {
      after([parentOf(node.path)], parentOf(node.path))
      toast(t('toast.trashed', { name: node.name }), 'info', {
        label: t('toast.undo'),
        fn: () => withSudo(serverId, t('sudo.restore', { name: node.name }), () => RestoreEntry(serverId, item)).then(p => {
          after([parentOf(p)], p, true)
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
        last = await withSudo(serverId, t('sudo.upload', { dir: pathArg(root, dir) }), () => withProgress(
          (done, total) => setUploads(u => u.map(x => x.key === it.path ? { ...x, p: total ? done / total : 1 } : x)),
          key => {
            setUploads(u => u.some(x => x.key === it.path) ? u : [...u, { key: it.path, dir, name: it.name, isDir: it.dir, p: 0 }])
            return Upload(serverId, it.path, dir, keep, key)
          }))
        count++
      } catch (e) {
        fail(e)
      } finally {
        setUploads(u => u.filter(x => x.key !== it.path))
      }
    }
    await after([dir], last || undefined, true)
    if (count) toast(t('upload.sent', { count, dir: pathArg(root, dir) }), 'ok')
  }
  const pickUpload = (dir: string) => PickUploadFiles().then(p => uploadTo(dir, p ?? []), fail)

  const download = (n: Node) => {
    const pt = progressToast(t('upload.receiving', { name: n.name }))
    withSudo(serverId, t('sudo.read', { name: n.name }), () => withProgress((done, total) => pt.set(total ? done / total : 1), key => Download(serverId, n.path, key))).then(
      local => local ? pt.done(t('upload.received', { path: local })) : pt.done(t('connect.cancel'), 'info'),
      e => pt.done(errText(e), 'warn'),
    )
  }
  const showProps = (n: Node) => withSudo(serverId, t('sudo.read', { name: n.name || n.path }), () => Stat(serverId, n.path)).then(pr => setProps({ node: n, props: pr }), fail)

  // path をツリーで選ぶ。いちばん上の外ならホームか / に移し、途中のフォルダを開く。読めないフォルダがあれば sudo モードを聞く
  const under = (p: string, d: string) => d === '/' || p === d || p.startsWith(d + '/')
  const reveal = async (path: string, openIt: boolean) => {
    const base = under(path, top) ? top : under(path, root) ? root : '/'
    const chain = [base]
    for (let p = parentOf(path); p !== base && under(p, base); p = parentOf(p)) chain.splice(1, 0, p)
    if (openIt && path !== base) chain.push(path)
    for (const d of chain) {
      const err = await load(d, true)
      if (err) {
        if (!isDenied(err)) fail(err)
        return
      }
    }
    setTop(base)
    setOpen(o => ({ ...o, ...Object.fromEntries(chain.map(d => [d, true])) }))
    setSelected(path)
    flashRow(path)
    scrollTo.current = path
  }

  // ほかの窓から渡されたパスをツリーで選ぶ。show でなければ (ゴミ箱から戻したとき)、戻した先のフォルダを読んでいるときだけ選ぶ
  useEffect(() => {
    const onReveal = (e: Event) => {
      const d = (e as CustomEvent<{ serverId: string; path: string; show?: boolean; open?: boolean }>).detail
      if (d.serverId !== serverId) return
      if (d.show) reveal(d.path, !!d.open)
      else if (dirs[parentOf(d.path)]) after([parentOf(d.path)], d.path, true)
    }
    window.addEventListener('drag:tree-reveal', onReveal)
    return () => window.removeEventListener('drag:tree-reveal', onReveal)
  })

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
      ? { icon: 'terminal', label: t('menu.cd'), fn: () => { setMTab('terminal'); terminalOf(serverId)?.cd(pathArg(root, n.path)) } }
      : { icon: 'terminal', label: t('menu.insertPath'), fn: () => { setMTab('terminal'); terminalOf(serverId)?.insert(pathArg(root, n.path)) } },
    { sep: true },
    n.dir && { icon: 'filePlus', label: t('menu.newFile'), fn: () => create(n.path, false) },
    n.dir && { icon: 'folderPlus', label: t('menu.newFolder'), fn: () => create(n.path, true) },
    // PC のファイルを選ぶ窓と保存先の窓は PC にしか出せないので、スマホの画面には出さない
    n.dir && !isRemote && { icon: 'upload', label: t('menu.uploadHere'), fn: () => pickUpload(n.path) },
    { sep: true },
    { icon: 'cut', label: t('menu.cut'), hint: 'Ctrl+X', disabled: n.top, fn: () => setClip({ path: n.path, move: true }) },
    // ホームはコピーだけできる
    { icon: 'copy', label: t('menu.copy'), hint: 'Ctrl+C', disabled: n.top && n.path !== root, fn: () => setClip({ path: n.path, move: false }) },
    { icon: 'paste', label: t('menu.paste'), hint: 'Ctrl+V', disabled: !clip, fn: () => paste(n.dir ? n.path : parentOf(n.path)) },
    { sep: true },
    { icon: 'edit', label: t('menu.rename'), hint: 'F2', disabled: n.top, fn: () => startRename(n) },
    { icon: 'trash', label: t('menu.delete'), hint: 'Delete', disabled: n.top, danger: true, fn: () => trash(n) },
    { sep: true },
    { icon: 'link', label: t('menu.copyPath'), fn: () => copyPath(n.path) },
    !isRemote && { icon: 'download', label: n.dir ? t('menu.downloadZip') : t('menu.download'), disabled: n.path === '/', fn: () => download(n) },
    n.path === top && { icon: 'trash', label: t('menu.openTrash'), fn: () => openTool(serverId, 'trash') },
    { icon: 'info', label: t('menu.props'), fn: () => showProps(n) },
  ]

  // エクスプローラーと同じキー。↑↓ で選ぶ行を動かし、→← でフォルダを開け閉めする
  const onKey = (e: React.KeyboardEvent) => {
    if (renaming) return
    const ctrl = e.ctrlKey || e.metaKey
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key
    if (!selected) {
      if (ctrl && key === 'v') {
        e.preventDefault()
        paste(top)
      }
      return
    }
    const n = nodeOf(selected)
    const rowPaths = () => [...box.current!.querySelectorAll<HTMLElement>('.tree-row[data-path]')].map(r => r.dataset.path!)
    const select = (p: string) => {
      setSelected(p)
      scrollTo.current = p
    }
    if (key === 'F2' && !n.top) startRename(n)
    else if (key === 'Delete' && !n.top) trash(n)
    else if (ctrl && key === 'c' && (!n.top || n.path === root)) setClip({ path: n.path, move: false })
    else if (ctrl && key === 'x' && !n.top) setClip({ path: n.path, move: true })
    else if (ctrl && key === 'v') paste(n.dir ? n.path : parentOf(n.path))
    else if (key === 'Enter') {
      if (!n.dir) openFile(serverId, n)
      else if (n.path !== top) toggle(n.path)
    } else if (key === 'ArrowUp' || key === 'ArrowDown') {
      const rows = rowPaths()
      const i = rows.indexOf(n.path)
      const next = rows[Math.max(0, Math.min(rows.length - 1, i + (key === 'ArrowUp' ? -1 : 1)))]
      if (next) select(next)
    } else if (key === 'ArrowRight') {
      if (n.dir && !open[n.path] && n.path !== top) toggle(n.path)
    } else if (key === 'ArrowLeft') {
      if (n.dir && open[n.path] && n.path !== top) toggle(n.path)
      else if (parentOf(n.path) !== top && n.path.startsWith(top === '/' ? '/' : top + '/')) select(parentOf(n.path))
    } else return
    e.preventDefault()
  }

  // フォルダを開け閉めする。はじめて開くときに中を読む
  const toggle = (path: string) => {
    const next = !open[path]
    setOpen(o => ({ ...o, [path]: next }))
    if (next && !dirs[path]?.entries) load(path, true)
  }

  const click = (e: Entry) => {
    if (longPressed.current) {
      longPressed.current = false
      return
    }
    setSelected(e.path)
    if (e.dir) toggle(e.path)
  }
  const contextMenu = (ev: React.MouseEvent, n: Node) => {
    ev.preventDefault()
    ev.stopPropagation()
    setSelected(n.path)
    setMenu({ x: ev.clientX, y: ev.clientY, node: n })
  }
  // スマホでは常に出し、PC ではマウスを乗せた行にだけ出す
  const moreButton = (n: Node) => (
    <button
      className={mobile ? 'grid h-9 w-9 flex-none place-items-center rounded-md text-muted' : `${navBtn} invisible group-hover:visible`}
      title={t('menu.more')}
      onClick={ev => {
        ev.stopPropagation()
        const r = ev.currentTarget.getBoundingClientRect()
        setSelected(n.path)
        setMenu({ x: r.left, y: r.bottom + 4, node: n })
      }}
    ><Icon name="more" /></button>
  )
  // 指で長押ししたらメニューを出す。そのあとの click では開け閉めしない
  const longPress = (n: Node) => (ev: React.PointerEvent) => {
    if (ev.pointerType !== 'touch') return
    const row = ev.currentTarget
    const { clientX: x, clientY: y } = ev
    const timer = setTimeout(() => {
      longPressed.current = true
      setSelected(n.path)
      setMenu({ x, y, node: n })
    }, 480)
    const cancel = () => clearTimeout(timer)
    row.addEventListener('pointerup', cancel, { once: true })
    row.addEventListener('pointercancel', cancel, { once: true })
    row.addEventListener('pointermove', cancel, { once: true })
  }

  // PC からのドラッグ中は、落とす先のフォルダの行を緑で囲む。ファイルの行ならそのファイルのあるフォルダ。
  // ツリーの行をドラッグしているときは、そのフォルダへ移す。自分の中と、いまあるフォルダには移さない
  const canMoveTo = (dir: string) => {
    const p = dragging.current
    return !!p && dir !== p && dir !== parentOf(p) && !dir.startsWith(p + '/')
  }
  // 行の中の動きは行だけが受け取り、行の無いところの動きはツリー全体 (いちばん上のフォルダ) が受け取る
  const mine = (ev: React.DragEvent) => (ev.target as HTMLElement).closest('[data-drop-dir]') === ev.currentTarget
  const dropProps = (dir: string) => ({
    'data-drop-dir': dir,
    onDragOver: (ev: React.DragEvent) => {
      if (!mine(ev)) return
      if (ev.dataTransfer.types.includes('Files')) setDropDir(dir)
      else if (canMoveTo(dir)) {
        ev.preventDefault()
        ev.dataTransfer.dropEffect = 'move'
        setDropDir(dir)
      }
    },
    onDragLeave: (ev: React.DragEvent) => { if (mine(ev)) setDropDir(d => d === dir ? '' : d) },
    onDrop: (ev: React.DragEvent) => {
      if (!mine(ev)) return
      setDropDir('')
      if (!canMoveTo(dir)) return
      ev.preventDefault()
      move(dragging.current, dir)
    },
  })
  const dropOn = 'bg-[rgba(63,207,142,.14)] shadow-[inset_0_0_0_1px_var(--ok)]'

  const pending = (path: string, depth: number) => uploads.filter(u => u.dir === path).map(u => (
    <div key={'up:' + u.key} className={`relative flex items-center gap-[5px] pr-1 whitespace-nowrap text-muted ${rowSize}`} style={{ paddingLeft: indent(depth) }}>
      <span className="w-[13px] flex-none" />
      <span className={u.isDir ? 'f-dir' : 'f-file'}><Icon name={u.isDir ? 'folder' : 'file'} size={15} /></span>
      <span className="min-w-0 flex-1 overflow-hidden text-ellipsis">{u.name}</span>
      <div className="h-1 w-16 flex-none overflow-hidden rounded-sm bg-line"><i className="block h-full bg-gradient-to-r from-ok to-[#7fe6b8]" style={{ width: `${u.p * 100}%` }} /></div>
      <span className="pl-2 font-mono text-[11px] text-faint">{Math.round(u.p * 100)}%</span>
    </div>
  ))

  const rowSize = mobile ? 'h-10 text-[14.5px]' : 'h-6 text-[13px]'
  const indent = (depth: number) => mobile ? `calc(14px + ${depth} * 16px)` : `calc(10px + ${depth} * 14px)`
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
          className={`tree-row group relative flex cursor-pointer items-center gap-[5px] pr-1 whitespace-nowrap select-none ${rowSize} ${flash === e.path ? 'flash' : ''} hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${dropDir && dropDir === (e.dir ? e.path : path) ? dropOn : selected === e.path ? 'bg-accent-soft shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--accent)_35%,transparent)]' : ''} ${cut ? 'opacity-50' : ''}`}
          style={{ paddingLeft: indent(depth), ['--d' as string]: depth }}
          data-d={depth}
          data-path={e.path}
          {...dropProps(e.dir ? e.path : path)}
          onClick={() => click(e)}
          onDoubleClick={() => { if (!e.dir) openFile(serverId, e) }}
          onPointerDown={mobile ? longPress(nodeOf(e.path)) : undefined}
          draggable={!mobile && renaming !== e.path}
          onDragStart={ev => {
            dragging.current = e.path
            ev.dataTransfer.setData('application/x-drag-path', JSON.stringify({ path: e.path, name: e.name, dir: e.dir }))
            if (e.dir) ev.dataTransfer.setData('application/x-drag-dir', '1')
            // デスクトップで開けないもの
            if (fileKind(e.name, e.dir) === 'bin') ev.dataTransfer.setData('application/x-drag-bin', '1')
            ev.dataTransfer.effectAllowed = 'copyMove'
          }}
          onDragEnd={() => {
            dragging.current = ''
            setDropDir('')
          }}
          onContextMenu={ev => contextMenu(ev, nodeOf(e.path))}
          title={e.path}
        >
          {e.dir
            ? <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
            : <span className="w-[13px] flex-none" />}
          <span className={color}><Icon name={icon} size={15} /></span>
          {renaming === e.path
            ? <RenameInput name={e.name} dir={e.dir} onDone={name => rename(nodeOf(e.path), name)} />
            : <span className="min-w-0 flex-1 overflow-hidden text-ellipsis">{e.name}</span>}
          {e.locked && <span className="flex-none text-faint" title={t('sudo.locked')}><Icon name="lock" size={12} /></span>}
          {['arc', 'bin'].includes(fileKind(e.name, e.dir)) && <span className="pl-1 font-mono text-[11px] text-faint">{fmtSize(e.size)}</span>}
          {e.link && <span className="text-faint"><Icon name="link" size={12} /></span>}
          {moreButton(nodeOf(e.path))}
        </div>,
        ...(e.dir && isOpen ? rows(e.path, depth + 1) : []),
      ]
    }), ...pending(path, depth)]
  }

  // 見出しのボタンは、選んでいるフォルダ (ファイルならそのフォルダ) に作る。選んでいなければいちばん上
  const here = () => !selected ? top : nodeOf(selected).dir ? selected : parentOf(selected)
  return (
    <>
      {!mobile && (
        <div className="flex h-9 flex-none items-center gap-0.5 pr-1.5 pl-3.5 text-[11px] font-semibold tracking-[.08em] text-muted">
          <span className="flex-1">{t('files.title')}</span>
          <button className={headBtn} title={t('menu.newFile')} onClick={() => create(here(), false)}><Icon name="filePlus" /></button>
          <button className={headBtn} title={t('menu.newFolder')} onClick={() => create(here(), true)}><Icon name="folderPlus" /></button>
          {!isRemote && <button className={headBtn} title={t('files.upload')} onClick={() => pickUpload(here())}><Icon name="upload" /></button>}
          <button className={headBtn} title={t('files.collapse')} onClick={() => setOpen({ [top]: true })}><Icon name="collapse" /></button>
          <button className={headBtn} title={t('tools.trash')} onClick={() => openTool(serverId, 'trash')}><Icon name="trash" /></button>
        </div>
      )}
      <div
        ref={box}
        tabIndex={0}
        className={`min-h-0 flex-1 overflow-auto pb-5 outline-none ${osDrag ? 'os-drag' : ''}`}
        onKeyDown={onKey}
        onContextMenu={ev => contextMenu(ev, nodeOf(top))}
        {...dropProps(top)}
      >
        <div
          className={`group mb-0.5 flex items-center gap-1.5 font-mono text-[11.5px] text-muted ${mobile ? 'h-[38px] pr-1 pl-4' : 'h-[26px] px-2.5'} ${dropDir === top ? dropOn : selected === top ? 'bg-accent-soft' : ''}`}
          title={top + '\n' + t('upload.dropHint')}
          data-tree-top={top}
          {...dropProps(top)}
          onClick={() => setSelected(top)}
          onContextMenu={ev => contextMenu(ev, nodeOf(top))}
        >
          {top !== '/' && <button className={navBtn} title={t('files.up')} onClick={e => { e.stopPropagation(); goTo(parentOf(top)) }}><Icon name="up" size={13} /></button>}
          {top !== root && <button className={navBtn} title={t('files.home')} onClick={e => { e.stopPropagation(); goTo(root) }}><Icon name="home" size={13} /></button>}
          <Icon name="folder" size={14} className="f-dir" />
          <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{top}</span>
          {moreButton(nodeOf(top))}
        </div>
        {rows(top, 0)}
        {(osDrag || (dragging.current && dropDir)) && (
          <div className="tree-drop">
            {!dropDir ? t('upload.dropHere') : t(osDrag ? 'upload.dropTo' : 'upload.moveTo', { dir: pathArg(root, dropDir) })}
          </div>
        )}
        {menu && <Menu x={menu.x} y={menu.y} title={pathArg(root, menu.node.path)} items={menuItems(menu.node)} onClose={() => setMenu(null)} />}
        {conflict && <ConflictDialog path={conflict.path} more={conflict.more} onAnswer={conflict.resolve} />}
        {props && <PropsDialog name={props.node.name} path={props.node.path} kind={fileKind(props.node.name, props.props.mode.startsWith('d'))} props={props.props} onClose={() => setProps(null)} />}
      </div>
    </>
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

const headBtn = 'grid h-[26px] w-[26px] place-items-center rounded-md text-muted hover:bg-[color-mix(in_srgb,var(--text)_8%,transparent)] hover:text-text'
const navBtn = 'grid h-[22px] w-[22px] flex-none place-items-center rounded-[5px] text-muted hover:bg-[color-mix(in_srgb,var(--text)_8%,transparent)] hover:text-text'

function Note({ depth, text, err = false }: { depth: number; text: string; err?: boolean }) {
  const mobile = useMobile()
  return (
    <div className={`truncate text-[12px] ${mobile ? 'h-10 leading-10' : 'h-6 leading-6'} ${err ? 'text-err' : 'text-faint'}`} style={{ paddingLeft: mobile ? `calc(34px + ${depth} * 16px)` : `calc(28px + ${depth} * 14px)` }}>
      {text}
    </div>
  )
}
