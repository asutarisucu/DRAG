import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListDir } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { Icon, type IconName } from './Icon'

type Entry = remote.Entry
type Dir = { entries?: Entry[]; error?: string; loading: boolean }

// 種類ごとのアイコンと色。mock/app.js の fileKind と同じ分け方
function kindOf(e: Entry): [IconName, string] {
  if (e.dir) return ['folder', 'f-dir']
  const n = e.name.toLowerCase()
  if (n.endsWith('.log')) return ['log', 'f-log']
  if (/\.(png|jpe?g|gif|webp)$/.test(n)) return ['image', 'f-img']
  if (/\.(gz|tgz|tar|zip|jar|7z)$/.test(n)) return ['archive', 'f-arc']
  if (/\.(sh|json|ya?ml|toml|properties|js|ts|py|env|conf|ini)$/.test(n)) return ['code', 'f-code']
  return ['file', 'f-file']
}

// 左のファイルツリー。フォルダは開いたときに中を読む
export function FileTree({ serverId, root }: { serverId: string; root: string }) {
  const { t } = useTranslation()
  const [dirs, setDirs] = useState<Record<string, Dir>>({})
  const [open, setOpen] = useState<Record<string, boolean>>({ [root]: true })
  const [selected, setSelected] = useState('')

  const load = useCallback((path: string) => {
    setDirs(d => ({ ...d, [path]: { ...d[path], loading: true } }))
    ListDir(serverId, path).then(
      entries => setDirs(d => ({ ...d, [path]: { entries: entries ?? [], loading: false } })),
      err => setDirs(d => ({ ...d, [path]: { error: String(err), loading: false } })),
    )
  }, [serverId])

  useEffect(() => { load(root) }, [load, root])

  const toggle = (e: Entry) => {
    setSelected(e.path)
    if (!e.dir) return
    const next = !open[e.path]
    setOpen(o => ({ ...o, [e.path]: next }))
    if (next && !dirs[e.path]?.entries) load(e.path)
  }

  const rows = (path: string, depth: number): React.ReactNode[] => {
    const d = dirs[path]
    if (!d || (d.loading && !d.entries)) return []
    if (d.error) return [<Note key={path + ':err'} depth={depth} text={t('files.error', { error: d.error })} err />]
    if (d.entries!.length === 0) return [<Note key={path + ':empty'} depth={depth} text={t('files.empty')} />]
    return d.entries!.flatMap(e => {
      const [icon, color] = kindOf(e)
      const isOpen = !!open[e.path]
      return [
        <div
          key={e.path}
          className={`tree-row relative flex h-6 cursor-pointer items-center gap-[5px] pr-1 text-[13px] whitespace-nowrap select-none hover:bg-[color-mix(in_srgb,var(--text)_4%,transparent)] ${selected === e.path ? 'bg-accent-soft shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--accent)_35%,transparent)]' : ''}`}
          style={{ paddingLeft: `calc(10px + ${depth} * 14px)`, ['--d' as string]: depth }}
          data-d={depth}
          onClick={() => toggle(e)}
          title={e.path}
        >
          {e.dir
            ? <span className={`text-faint transition-transform ${isOpen ? 'rotate-90' : ''}`}><Icon name="chevron" size={13} /></span>
            : <span className="w-[13px] flex-none" />}
          <span className={color}><Icon name={icon} size={15} /></span>
          <span className="min-w-0 flex-1 overflow-hidden text-ellipsis">{e.name}</span>
          {e.link && <span className="text-faint"><Icon name="link" size={12} /></span>}
        </div>,
        ...(e.dir && isOpen ? rows(e.path, depth + 1) : []),
      ]
    })
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto pb-5">
      <div className="mb-0.5 flex h-[26px] items-center gap-1.5 px-2.5 font-mono text-[11.5px] text-muted" title={root}>
        <Icon name="folder" size={14} className="f-dir" />
        <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">{root}</span>
      </div>
      {rows(root, 0)}
    </div>
  )
}

function Note({ depth, text, err = false }: { depth: number; text: string; err?: boolean }) {
  return (
    <div className={`h-6 truncate text-[12px] leading-6 ${err ? 'text-err' : 'text-faint'}`} style={{ paddingLeft: `calc(28px + ${depth} * 14px)` }}>
      {text}
    </div>
  )
}
