import { ShellInput } from '../wailsjs/go/main/App'
import i18n from './i18n'
import { toast } from './toasts'

// キー入力をシェルへ送る。Wails は呼び出しを並行して動かすので、続けて呼ぶと順番が入れ替わることがある。
// シェルごとに1つずつ送り、送っている間に打たれた分はまとめて次に送る
const queues = new Map<string, { busy: boolean; buf: string }>()

// 端末が文字として扱わないもの (C0 の制御文字と DEL)。引用符で囲んでも、シェルが読む前に端末や行の編集が受け取る
const CONTROL = /[\x00-\x1f\x7f]/

// typeLine は、サーバーから来た名前やパスを入れたコマンドを、利用者の代わりにシェルに打つ。enter なら Enter も押す。
// 名前に制御文字があると、行を消したり区切ったりして、別のコマンドを動かせてしまうので、打たずに false を返す
export function typeLine(shellId: string, text: string, enter: boolean) {
  if (CONTROL.test(text)) {
    toast(i18n.t('toast.controlChars'), 'warn')
    return false
  }
  sendInput(shellId, enter ? text + '\r' : text)
  return true
}

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
