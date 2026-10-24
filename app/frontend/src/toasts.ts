import { useSyncExternalStore } from 'react'

// 右下に数秒だけ出す知らせ。action を付けると「元に戻す」のようなボタンを並べる
export type Toast = {
  id: number
  text: string
  kind: 'info' | 'ok' | 'warn'
  action?: { label: string; fn: () => void }
}

let list: Toast[] = []
let next = 1
const listeners = new Set<() => void>()
const emit = () => listeners.forEach(l => l())

export function dismiss(id: number) {
  list = list.filter(t => t.id !== id)
  emit()
}

export function toast(text: string, kind: Toast['kind'] = 'info', action?: Toast['action']) {
  const id = next++
  list = [...list, { id, text, kind, action }]
  emit()
  setTimeout(() => dismiss(id), action ? 6000 : 2800)
}

export function useToasts(): Toast[] {
  return useSyncExternalStore(l => {
    listeners.add(l)
    return () => listeners.delete(l)
  }, () => list)
}
