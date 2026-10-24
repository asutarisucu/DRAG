import { useSyncExternalStore } from 'react'
import type { IconName } from './components/Icon'

// デスクトップのウィンドウ。サーバーごとに並び順を持ち、その順でタスクバーに並べる
export type Rect = { x: number; y: number; w: number; h: number }
export type Win = {
  id: string
  serverId: string
  kind: 'session' | 'terminal' | 'editor' | 'log' | 'image' | 'archive' | 'monitor' | 'services' | 'docker' | 'trash'
  path?: string // ファイルのウィンドウで開いているファイル
  title: string
  icon: IconName
  shellId: string
  rect: Rect
  prev?: Rect // 最大化や半分に寄せる前の位置
  min: boolean
  max: boolean
  snapped: boolean
  z: number
  ended: boolean
  detachable: boolean // screen と tmux。閉じてもセッションは残る
  replayFromIdle: boolean // 移したコマンドを打った行から描き直す
  sessionKey?: string // screen か tmux のセッションなら "screen:名前" の形
  // ほかの端末で開いているシェルを見ているときの、そのシェルの大きさ。閉じてもシェルは閉じない
  shared?: { cols: number; rows: number }
  view?: string // リソース監視で出している詳細 (cpu・mem・disk・net)。概要なら空
}

let wins: Win[] = []
// 閉じる前に確かめることがあるウィンドウ (保存していないエディターなど)。false を返したら閉じない
const guards = new Map<string, () => Promise<boolean>>()

export function setCloseGuard(id: string, fn: (() => Promise<boolean>) | null) {
  if (fn) guards.set(id, fn)
  else guards.delete(id)
}

export async function canClose(id: string) {
  const g = guards.get(id)
  return g ? g() : true
}
let seq = 0
let zTop = 1
const cascade = new Map<string, number>()
const listeners = new Set<() => void>()
const emit = () => listeners.forEach(l => l())

export function openWin(o: Omit<Win, 'id' | 'rect' | 'min' | 'max' | 'snapped' | 'z' | 'ended'> & { rect?: Partial<Rect> }, host: { w: number; h: number }) {
  const id = `w${++seq}`
  const n = (cascade.get(o.serverId) ?? 0)
  cascade.set(o.serverId, n + 1)
  const w = Math.min(o.rect?.w ?? 540, host.w - 16), h = Math.min(o.rect?.h ?? 320, host.h - 16)
  const x = Math.max(0, Math.min(o.rect?.x ?? 40 + (n % 6) * 28, host.w - w))
  const y = Math.max(0, Math.min(o.rect?.y ?? 24 + (n % 6) * 24, host.h - h))
  wins = [...wins, { ...o, id, rect: { x, y, w, h }, min: false, max: false, snapped: false, z: ++zTop, ended: false }]
  emit()
  return id
}

export function updateWin(id: string, patch: Partial<Win>) {
  wins = wins.map(w => w.id === id ? { ...w, ...patch } : w)
  emit()
}

export function focusWin(id: string) {
  updateWin(id, { z: ++zTop, min: false })
}

export function closeWin(id: string) {
  wins = wins.filter(w => w.id !== id)
  emit()
}

export function winById(id: string) {
  return wins.find(w => w.id === id)
}

export function winsOf(serverId: string) {
  return wins.filter(w => w.serverId === serverId)
}

export function winByShell(shellId: string) {
  return wins.find(w => w.shellId === shellId)
}

// タスクバーの並べ替え。サーバーの中での順番を from から to へ移す
export function reorderWin(serverId: string, from: number, to: number) {
  const mine = wins.filter(w => w.serverId === serverId)
  const others = wins.filter(w => w.serverId !== serverId)
  mine.splice(to, 0, mine.splice(from, 1)[0])
  wins = [...others, ...mine]
  emit()
}

// 前面のウィンドウ。最小化しているものは除く
export function topWin(serverId: string) {
  return wins.filter(w => w.serverId === serverId && !w.min).sort((a, b) => b.z - a.z)[0]
}

export function useWins(serverId: string): Win[] {
  const all = useSyncExternalStore(l => {
    listeners.add(l)
    return () => listeners.delete(l)
  }, () => wins)
  return all.filter(w => w.serverId === serverId)
}
