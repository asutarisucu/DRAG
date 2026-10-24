import { useSyncExternalStore } from 'react'
import { DisableSudo } from '../wailsjs/go/main/App'

// sudo モードの ON と OFF をサーバーごとに持つ。パスワードは Go の側だけが持つ。
// ON が要る操作は requireSudo で理由を出してパスワードを聞き、ON になってから続ける
export type SudoAsk = { serverId: string; reason: string; resolve: (ok: boolean) => void }

let on: Record<string, boolean> = {}
let ask: SudoAsk | null = null
const listeners = new Set<() => void>()
const emit = () => listeners.forEach(l => l())
const subscribe = (l: () => void) => {
  listeners.add(l)
  return () => { listeners.delete(l) }
}

export const sudoOn = (serverId: string) => !!on[serverId]

export function setSudo(serverId: string, v: boolean) {
  on = { ...on, [serverId]: v }
  emit()
}

export function turnOffSudo(serverId: string) {
  DisableSudo(serverId)
  setSudo(serverId, false)
}

// OFF ならパスワードを聞く窓を出す。ON になれば true、やめたら false
export function requireSudo(serverId: string, reason = ''): Promise<boolean> {
  if (on[serverId]) return Promise.resolve(true)
  ask?.resolve(false)
  return new Promise(resolve => {
    ask = { serverId, reason, resolve: ok => { ask = null; emit(); resolve(ok) } }
    emit()
  })
}

// ログインしたユーザーの権限で、読めないか書けなくて失敗したか。SFTP は "permission denied"、コマンドは "Permission denied" を返す
export const isDenied = (e: unknown) => /permission denied/i.test(String(e))

// run が権限で失敗したら、sudo モードを ON にするかを聞き、ON になればもう一度動かす。
// 権限が足りているところでは聞かない (root でログインしているときなど)
export async function withSudo<T>(serverId: string, reason: string, run: () => Promise<T>): Promise<T> {
  try {
    return await run()
  } catch (e) {
    if (on[serverId] || !isDenied(e) || !(await requireSudo(serverId, reason))) throw e
    return run()
  }
}

export const useSudo = (serverId: string) => useSyncExternalStore(subscribe, () => !!on[serverId])
export const useSudoAsk = () => useSyncExternalStore(subscribe, () => ask)
