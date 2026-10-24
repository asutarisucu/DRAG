import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RestoreEntry, TrashDelete, TrashList } from '../../wailsjs/go/main/App'
import type { remote } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { KIND_ICON, fileKind } from '../fileKind'
import { isDenied, withSudo } from '../sudo'
import { Icon } from './Icon'
import { useFoot } from './Services'

const errText = (e: unknown) => String(e).replace(/^Error: /, '')
const parentOf = (p: string) => p.slice(0, p.lastIndexOf('/')) || '/'
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const pad = (n: number) => String(n).padStart(2, '0')

// 消した時刻。今日なら時刻だけ、前の日なら日付も出す
function when(sec: number) {
  const d = new Date(sec * 1000)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  return d.toDateString() === new Date().toDateString() ? hm : `${d.getMonth() + 1}/${d.getDate()} ${hm}`
}

// ゴミ箱の中。元に戻すか、完全に削除する。ゴミ箱はサーバーの ~/.local/share/Trash で、gio trash と同じもの
export function TrashView({ serverId, home }: { serverId: string; home: string }) {
  const { t } = useTranslation()
  const [list, setList] = useState<remote.TrashEntry[] | null>(null)
  const [error, setError] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [say, foot] = useFoot()
  const tilde = (p: string) => p === '/' ? '' : p === home ? '~' : p.startsWith(home + '/') ? '~' + p.slice(home.length) : p

  const load = () => TrashList(serverId).then(l => { setList(l ?? []); setError('') }, e => setError(errText(e)))
  useEffect(() => {
    load()
    return EventsOn('trash:changed', (id: string) => { if (id === serverId) load() })
  }, [serverId])

  const fail = (e: unknown) => say(isDenied(e) ? t('files.denied') : errText(e))
  const restore = (it: remote.TrashEntry) => {
    withSudo(serverId, t('sudo.restore', { name: baseOf(it.from) }), () => RestoreEntry(serverId, it)).then(p => {
      say(t('toast.restored', { name: baseOf(p) }))
      // ツリーで戻した先を開いて選ぶ
      window.dispatchEvent(new CustomEvent('drag:tree-reveal', { detail: { serverId, path: p } }))
    }, fail)
  }
  const remove = (names: string[], done: string) =>
    withSudo(serverId, t('trash.needSudo'), () => TrashDelete(serverId, names)).then(() => say(done), fail)

  if (!list) return <div className="tool-empty">{error || t('mon.waiting')}</div>
  const n = list.length
  return (
    <>
      <div className="win-toolbar">
        <span className="text-[11.5px] text-muted">{t('trash.count', { n })}</span>
        <div className="flex-1" />
        {confirming
          ? <>
            <span className="text-[11.5px] text-muted">{t('trash.confirmEmpty')}</span>
            <button className="sbtn" onClick={() => setConfirming(false)}>{t('trash.cancel')}</button>
            <button className="sbtn danger" onClick={() => { setConfirming(false); remove([], t('trash.emptied')) }}>{t('trash.emptyNow')}</button>
          </>
          : <button className="sbtn" disabled={!n} onClick={() => setConfirming(true)}>{t('trash.empty')}</button>}
      </div>
      {n
        ? (
          <div className="dk-list">
            {list.map(it => {
              const name = baseOf(it.from)
              const [icon, color] = KIND_ICON[fileKind(name, it.dir)]
              return (
                <div key={it.name} className="dk-row static" data-trash={it.name}>
                  <span className={color}><Icon name={icon} /></span>
                  <div className="dk-name">
                    <div className="font-mono text-[12.5px]">{name}</div>
                    <div className="font-mono text-[11.5px] text-muted">{t('trash.deletedAt', { dir: tilde(parentOf(it.from)), when: when(it.deleted) })}</div>
                  </div>
                  <div className="dk-acts">
                    <button className="sbtn" onClick={() => restore(it)}>{t('trash.restore')}</button>
                    <button className="sbtn" onClick={() => remove([it.name], t('trash.deleted', { name }))}>{t('trash.delete')}</button>
                  </div>
                </div>
              )
            })}
          </div>
        )
        : <div className="tool-empty">{t('trash.none')}</div>}
      {foot}
    </>
  )
}
