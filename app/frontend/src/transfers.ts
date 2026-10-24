import { EventsOn } from '../wailsjs/runtime/runtime'

// 送受信の進み具合。Go の側が "transfer:progress" (key, done, total) で知らせてくる
type Listener = (done: number, total: number) => void

const listeners = new Map<string, Listener>()
let seq = 0

EventsOn('transfer:progress', (key: string, done: number, total: number) => listeners.get(key)?.(done, total))

// key を作って fn に渡し、終わるまでの間だけ進み具合を受け取る
export async function withProgress<T>(onProgress: Listener, fn: (key: string) => Promise<T>): Promise<T> {
  const key = `x${++seq}`
  listeners.set(key, onProgress)
  try {
    return await fn(key)
  } finally {
    listeners.delete(key)
  }
}
