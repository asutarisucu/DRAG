import i18n from './i18n'
import { fileKind } from './fileKind'
import { toast } from './toasts'
import { focusWin, openWin, winsOf, type Win } from './windows'

const ARCHIVE_OK = /\.(zip|jar|tar|tgz|gz)$/i

// ファイルを種類に合ったウィンドウで開く。テキストは編集、ログは追従、画像は表示、圧縮ファイルは中を見る。
// もう開いていれば、そのウィンドウを前に出す
export function openFile(serverId: string, e: { path: string; name: string; dir: boolean }, pos?: { x: number; y: number }) {
  const k = fileKind(e.name, e.dir)
  if (k === 'dir') {
    toast(i18n.t('open.isDir', { name: e.name }))
    return
  }
  if (k === 'bin' || (k === 'arc' && !ARCHIVE_OK.test(e.name))) {
    toast(i18n.t('open.binary', { name: e.name }), 'warn')
    return
  }
  const open = winsOf(serverId).find(w => w.path === e.path)
  if (open) {
    focusWin(open.id)
    return
  }
  const [kind, icon, w, h]: [Win['kind'], Win['icon'], number, number] =
    k === 'log' ? ['log', 'log', 520, 320]
      : k === 'img' ? ['image', 'image', 340, 320]
        : k === 'arc' ? ['archive', 'archive', 560, 360]
          : ['editor', 'code', 540, 340]
  const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
  openWin({
    serverId, kind, title: e.name, icon, shellId: '', detachable: false, replayFromIdle: false, path: e.path,
    rect: { ...pos, w, h },
  }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
}
