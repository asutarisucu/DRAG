import { EventsOn } from '../wailsjs/runtime/runtime'

// ログの追記は、LogFollow が番号を返すより先に届くことがある。受け手が決まるまではためておく
type Handler = { out: (text: string) => void; end: (error: string) => void }
type Item = { text: string } | { error: string }

const handlers = new Map<string, Handler>()
const pending = new Map<string, Item[]>()

function give(h: Handler, it: Item) {
  if ('text' in it) h.out(it.text)
  else h.end(it.error)
}

function deliver(id: string, it: Item) {
  const h = handlers.get(id)
  if (h) give(h, it)
  else pending.set(id, [...(pending.get(id) ?? []), it])
}

EventsOn('log:out', (id: string, text: string) => deliver(id, { text }))
EventsOn('log:end', (id: string, error: string) => deliver(id, { error }))

export function attachLog(id: string, h: Handler) {
  handlers.set(id, h)
  for (const it of pending.get(id) ?? []) give(h, it)
  pending.delete(id)
  return () => { handlers.delete(id) }
}
