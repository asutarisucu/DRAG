import i18n from './i18n'
import type { IconName } from './components/Icon'
import { setMTab } from './mobile'
import { focusWin, openWin, updateWin, winsOf, type Win } from './windows'

// リソース監視・サービス・Docker・ゴミ箱のウィンドウ。サーバーごとに1つずつ開き、開いていれば前に出す。
// ゴミ箱はタスクバーに置かず、ツリーの上のボタンから開く
export type ToolKind = 'monitor' | 'services' | 'docker' | 'trash'

export const TOOLS: [ToolKind, IconName][] = [['monitor', 'chart'], ['services', 'server'], ['docker', 'box']]
const SIZE: Record<ToolKind, [number, number]> = { monitor: [600, 230], services: [600, 340], docker: [700, 420], trash: [520, 320] }

// サービスかコンテナのログをログのウィンドウで開く。path は "journal:名前" か "docker:名前" にする
export function openStreamLog(serverId: string, kind: 'journal' | 'docker', name: string) {
  setMTab('desktop')
  const path = `${kind}:${name}`
  const open = winsOf(serverId).find(w => w.path === path)
  if (open) {
    focusWin(open.id)
    return
  }
  const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
  openWin({
    serverId, kind: 'log', title: i18n.t('svc.logOf', { name }), icon: 'log', shellId: '', detachable: false, replayFromIdle: false, path,
    rect: { w: 560, h: 320 },
  }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
}

export function openTool(serverId: string, kind: ToolKind, view = '') {
  setMTab('desktop')
  const open = winsOf(serverId).find(w => w.kind === kind)
  if (open) {
    focusWin(open.id)
    if (view) updateWin(open.id, { view })
    return
  }
  // リソース監視の詳細は表が入るので、はじめから高くする
  const [w, h] = kind === 'monitor' && view ? [600, 460] : SIZE[kind]
  const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
  openWin({
    serverId, kind, title: i18n.t(`tools.${kind}`), icon: kind === 'trash' ? 'trash' : TOOLS.find(x => x[0] === kind)![1], shellId: '',
    detachable: false, replayFromIdle: false, view, rect: { w, h },
  } as Omit<Win, 'id' | 'rect' | 'min' | 'max' | 'snapped' | 'z' | 'ended'>, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
}
