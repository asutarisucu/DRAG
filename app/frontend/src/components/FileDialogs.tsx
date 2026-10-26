import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DirUsage } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { fmtSize } from '../toasts'
import { KIND_ICON, KIND_LABEL, type Kind } from '../fileKind'
import { Icon } from './Icon'
import { Modal } from './Modal'

const btn = 'btn'

export type ConflictAnswer = { choice: 'skip' | 'keep' | 'overwrite'; all: boolean }

// アップロードで同じ名前があったときに、スキップ・両方残す・上書きを選んでもらう
export function ConflictDialog({ path, more, onAnswer }: { path: string; more: boolean; onAnswer: (a: ConflictAnswer) => void }) {
  const { t } = useTranslation()
  const [all, setAll] = useState(false)
  const choose = (choice: ConflictAnswer['choice']) => onAnswer({ choice, all })
  return (
    <Modal title={t('upload.conflictTitle')} width={460} onClose={() => onAnswer({ choice: 'skip', all: false })}>
      <p className="mt-0 mb-3.5"><span className="font-mono">{t('upload.exists', { path })}</span></p>
      {more && (
        <label className="inline-flex cursor-pointer items-center gap-1.5 text-muted select-none">
          <input type="checkbox" className="accent-[var(--accent)]" checked={all} onChange={e => setAll(e.target.checked)} />{t('upload.sameForRest')}
        </label>
      )}
      <div className="mt-[18px] flex items-center gap-2">
        <button className={btn} onClick={() => choose('skip')}>{t('upload.skip')}</button>
        <div className="flex-1" />
        <button className={btn} onClick={() => choose('keep')}>{t('upload.keep')}</button>
        <button className="btn-primary" onClick={() => choose('overwrite')} autoFocus>{t('upload.overwrite')}</button>
      </div>
    </Modal>
  )
}

// プロパティ。種類の名前はファイルツリーのアイコンと同じ分け方。
// フォルダは中のファイルを数えるので、大きさはあとから出す
export function PropsDialog({ serverId, name, path, kind, props, onClose }: {
  serverId: string
  name: string
  path: string
  kind: Kind
  props: remote.Props
  onClose: () => void
}) {
  const { t } = useTranslation()
  const dir = props.mode.startsWith('d')
  const [usage, setUsage] = useState<remote.Usage | null>(null)
  const [usageErr, setUsageErr] = useState('')
  useEffect(() => {
    if (dir) DirUsage(serverId, path).then(setUsage, e => setUsageErr(String(e).replace(/^Error: /, '')))
  }, [serverId, path, dir])
  const rows: [string, string][] = [
    [t('props.kind'), t(KIND_LABEL[kind])],
    [t('props.where'), path.slice(0, path.lastIndexOf('/')) || '/'],
    [t('props.size'), !dir ? fmtSize(props.size) : usage ? `${fmtSize(usage.size)} (${t('props.files', { n: usage.files })})` : usageErr || t('props.counting')],
    [t('props.owner'), `${props.owner}:${props.group}`],
    [t('props.mode'), props.mode],
    [t('props.modified'), new Date(props.modTime * 1000).toLocaleString()],
    ...(props.target ? [[t('props.target'), props.target] as [string, string]] : []),
  ]
  return (
    <Modal title={t('props.title')} width={460} onClose={onClose}>
      <div className="mb-3.5 flex items-center gap-2.5 text-[15px] font-medium text-strong">
        <Icon name={KIND_ICON[kind][0]} size={22} className={KIND_ICON[kind][1]} />
        <span className="break-all">{name || '/'}</span>
      </div>
      <div className="grid grid-cols-[90px_minmax(0,1fr)] gap-x-3.5 gap-y-2 text-[12.5px] [overflow-wrap:anywhere]">
        {rows.map(([k, v]) => [
          <span key={k + ':k'} className="text-muted">{k}</span>,
          <span key={k + ':v'} className="font-mono">{v}</span>,
        ])}
      </div>
    </Modal>
  )
}
