import { OpenShell } from '../wailsjs/go/main/App'
import { setMTab } from './mobile'
import { attachCommand, sessionKey, sessionKeyOf } from './sessions'
import { sendInput } from './shellInput'
import { terminalOf } from './terminals'
import { toast } from './toasts'
import { focusWin, openWin, winsOf } from './windows'

// screen か tmux のセッションをデスクトップのウィンドウで開く。
// もうウィンドウで開いていればそれを前に出し、右下のターミナルで開いていればそこから移す
// ほかのユーザーの screen (user がある) は、そのユーザーになって開く。sudo のパスワードはウィンドウの中で聞かれる
export async function openSessionWindow(serverId: string, s: { kind: string; name: string; user?: string }, pos?: { x: number; y: number }) {
  const key = sessionKey(s)
  setMTab('desktop')
  const open = winsOf(serverId).find(w => w.sessionKey === key)
  if (open) {
    focusWin(open.id)
    return
  }
  if (sessionKeyOf(terminalOf(serverId)?.foreground() ?? '') === key) {
    terminalOf(serverId)?.popOut(pos)
    return
  }
  const desk = document.querySelector<HTMLElement>(`[data-desktop="${serverId}"]`)?.getBoundingClientRect()
  try {
    const id = await OpenShell(serverId, 100, 30, '')
    openWin({
      serverId, kind: 'session', title: s.user ? `${s.user}/${s.name}` : s.name, icon: 'layers', shellId: id,
      detachable: true, replayFromIdle: false, sessionKey: key, rect: pos,
    }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
    sendInput(id, attachCommand(s) + '\r')
  } catch (e) {
    toast(String(e), 'warn')
  }
}
