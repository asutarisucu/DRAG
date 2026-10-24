import { ShellInput } from '../wailsjs/go/main/App'

// キー入力をシェルへ送る。Wails は呼び出しを並行して動かすので、続けて呼ぶと順番が入れ替わることがある。
// シェルごとに1つずつ送り、送っている間に打たれた分はまとめて次に送る
const queues = new Map<string, { busy: boolean; buf: string }>()

export function sendInput(shellId: string, data: string) {
  let q = queues.get(shellId)
  if (!q) {
    q = { busy: false, buf: '' }
    queues.set(shellId, q)
  }
  q.buf += data
  if (q.busy) return
  const flush = () => {
    const chunk = q!.buf
    q!.buf = ''
    if (!chunk) {
      q!.busy = false
      return
    }
    q!.busy = true
    ShellInput(shellId, chunk).finally(flush)
  }
  flush()
}
