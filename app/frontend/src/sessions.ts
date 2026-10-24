import type { remote } from '../wailsjs/go/models'

// screen と tmux のセッションの扱い

// シェルに打つ形に引用符で囲む
const quote = (v: string) => /^[\w.-]+$/.test(v) ? v : `'${v.replace(/'/g, `'\\''`)}'`

// セッションを開くコマンド。screen は -x にして、ほかで開いている間でも同じ画面を共有して開く。
// ほかのユーザーの screen は、sudo でそのユーザーになって開く。screen は端末の持ち主を確かめるが、
// sudo の use_pty (Ubuntu の既定) は、そのユーザーの持ち物の端末を作るので通る。
// script で端末を作ると、ウィンドウを閉じたときに script が Ctrl+D を中へ送り、セッションの中のシェルが終わってしまう
export function attachCommand(s: { kind: string; name: string; user?: string }) {
  if (s.kind === 'screen' && s.user) return `sudo -u ${quote(s.user)} screen -x ${quote(s.name)}`
  return s.kind === 'screen' ? `screen -x ${quote(s.name)}` : `tmux attach -t ${quote(s.name)}`
}

// セッションを指す "screen:名前" の形。screen の名前の頭の PID は外す。ほかのユーザーのものは "screen:ユーザー/名前"
export function sessionKey(s: { kind: string; name: string; user?: string }) {
  return `${s.kind}:${s.user ? s.user + '/' : ''}${s.kind === 'screen' ? s.name.replace(/^\d+\./, '') : s.name}`
}

// コマンドの行から "screen:名前" か "tmux:名前" を取り出す。名前の分からないものは空
export function sessionKeyOf(cmdline: string) {
  const w = cmdline.trim().split(/\s+/)
  const cmd = w[0]?.split('/').pop()
  // ほかのユーザーの screen を開くコマンド (sudo -u ユーザー screen -x 名前)
  if (cmd === 'sudo' && w[1] === '-u' && w[3]?.split('/').pop() === 'screen') {
    const x = w.findIndex((v, j) => j > 3 && ['-x', '-r'].includes(v))
    return x > 0 && w[x + 1] ? `screen:${w[2]}/${w[x + 1].replace(/^\d+\./, '')}` : ''
  }
  const after = (flags: string[]) => {
    const i = w.findIndex(x => flags.includes(x))
    return i >= 0 ? w[i + 1] ?? '' : ''
  }
  if (cmd === 'screen') {
    const n = after(['-S', '-r', '-x', '-R', '-dr', '-rd', '-Dr', '-rD'])
    return n ? `screen:${n.replace(/^\d+\./, '')}` : ''
  }
  if (cmd === 'tmux') {
    const n = ['new', 'new-session'].includes(w[1]) ? after(['-s']) : ['attach', 'a', 'at', 'attach-session'].includes(w[1]) ? after(['-t']) : ''
    return n ? `tmux:${n}` : ''
  }
  return ''
}

// 一覧の1件と "screen:名前" が同じセッションか
export function sameSession(s: remote.Session, key: string | undefined) {
  return !!key && sessionKey(s) === key.replace(/^(screen:(?:[^/]+\/)?)\d+\./, '$1')
}
