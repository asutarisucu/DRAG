import { useSyncExternalStore } from 'react'

// 右下に数秒だけ出す知らせ。action を付けると「元に戻す」のようなボタンを並べる。
// progress があると進み具合の棒を出し、終わるまで消さない
export type Toast = {
  id: number
  text: string
  kind: 'info' | 'ok' | 'warn'
  action?: { label: string; fn: () => void }
  progress?: number
  leaving?: boolean // 消える動きの間
}

let list: Toast[] = []
let next = 1
const listeners = new Set<() => void>()
const emit = () => listeners.forEach(l => l())

// 消すときは、少し下へずらしながら薄くしてから外す
export function dismiss(id: number) {
  if (!list.some(t => t.id === id && !t.leaving)) return
  patch(id, { leaving: true })
  setTimeout(() => {
    list = list.filter(t => t.id !== id)
    emit()
  }, 200)
}

function patch(id: number, p: Partial<Toast>) {
  list = list.map(t => t.id === id ? { ...t, ...p } : t)
  emit()
}

export function toast(text: string, kind: Toast['kind'] = 'info', action?: Toast['action']) {
  const id = next++
  list = [...list, { id, text, kind, action }]
  emit()
  setTimeout(() => dismiss(id), action ? 6000 : 2800)
}

// 進み具合を出す知らせ。終わったら done で文を変え、少しして消す
export function progressToast(text: string) {
  const id = next++
  list = [...list, { id, text, kind: 'info', progress: 0 }]
  emit()
  return {
    set: (p: number) => patch(id, { progress: p }),
    done: (text: string, kind: Toast['kind'] = 'ok') => {
      patch(id, { text, kind, progress: undefined })
      setTimeout(() => dismiss(id), 2400)
    },
  }
}

export function useToasts(): Toast[] {
  return useSyncExternalStore(l => {
    listeners.add(l)
    return () => listeners.delete(l)
  }, () => list)
}

export function fmtSize(b: number) {
  if (b < 1024) return `${b} B`
  const u = ['KB', 'MB', 'GB']
  let v = b / 1024, i = 0
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${u[i]}`
}
