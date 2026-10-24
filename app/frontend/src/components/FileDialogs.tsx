import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { remote } from '../../wailsjs/go/models'
import { fmtSize } from '../toasts'
import { KIND_ICON, KIND_LABEL, type Kind } from '../fileKind'
import { Icon } from './Icon'
import { Modal } from './Modal'

const btn = 'h-8 rounded-lg border border-line-strong bg-raised px-3.5 hover:bg-btn-hover'

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
        <button className="btn-primary h-8 rounded-lg px-3.5" onClick={() => choose('overwrite')} autoFocus>{t('upload.overwrite')}</button>
      </div>
    </Modal>
  )
}

// プロパティ。種類の名前はファイルツリーのアイコンと同じ分け方
export function PropsDialog({ name, path, kind, props, onClose }: {
  name: string
  path: string
  kind: Kind
  props: remote.Props
  onClose: () => void
}) {
  const { t } = useTranslation()
  const dir = props.mode.startsWith('d')
  const rows: [string, string][] = [
    [t('props.kind'), t(KIND_LABEL[kind])],
    [t('props.where'), path.slice(0, path.lastIndexOf('/')) || '/'],
    ...(dir ? [] : [[t('props.size'), fmtSize(props.size)] as [string, string]]),
    [t('props.owner'), `${props.owner}:${props.group}`],
    [t('props.mode'), props.mode],
    [t('props.modified'), new Date(props.modTime * 1000).toLocaleString()],
    ...(props.target ? [[t('props.target'), props.target] as [string, string]] : []),
  ]
  return (
    <Modal title={t('props.title')} width={460} onClose={onClose}>
      <div className="mb-3.5 flex items-center gap-2.5 text-[15px] font-semibold">
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
