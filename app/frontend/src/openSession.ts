import { OpenShell } from '../wailsjs/go/main/App'
import { attachCommand, sameSession, sessionKeyOf } from './sessions'
import { sendInput } from './shellInput'
import { terminalOf } from './terminals'
import { toast } from './toasts'
import { focusWin, openWin, winsOf } from './windows'

// screen か tmux のセッションをデスクトップのウィンドウで開く。
// もうウィンドウで開いていればそれを前に出し、右下のターミナルで開いていればそこから移す
export async function openSessionWindow(serverId: string, s: { kind: string; name: string }, pos?: { x: number; y: number }) {
  const key = `${s.kind}:${s.name.replace(/^\d+\./, '')}`
  const open = winsOf(serverId).find(w => sameSession({ ...s, attached: false, desc: '' }, w.sessionKey))
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
      serverId, kind: 'session', title: s.name, icon: 'layers', shellId: id,
      detachable: true, replayFromIdle: false, sessionKey: key, rect: pos,
    }, { w: desk?.width ?? 800, h: desk?.height ?? 500 })
    sendInput(id, attachCommand(s) + '\r')
  } catch (e) {
    toast(String(e), 'warn')
  }
}
