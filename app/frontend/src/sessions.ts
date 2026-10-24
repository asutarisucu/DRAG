import type { remote } from '../wailsjs/go/models'

// screen と tmux のセッションの扱い

// セッションを開くコマンド。screen は -x にして、ほかで開いている間でも同じ画面を共有して開く
export function attachCommand(s: { kind: string; name: string }) {
  const q = /^[\w.-]+$/.test(s.name) ? s.name : `'${s.name.replace(/'/g, `'\\''`)}'`
  return s.kind === 'screen' ? `screen -x ${q}` : `tmux attach -t ${q}`
}

// コマンドの行から "screen:名前" か "tmux:名前" を取り出す。名前の分からないものは空
export function sessionKeyOf(cmdline: string) {
  const w = cmdline.trim().split(/\s+/)
  const cmd = w[0]?.split('/').pop()
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

// 一覧の1件と "screen:名前" が同じセッションか。screen の一覧の名前には PID が付く
export function sameSession(s: remote.Session, key: string | undefined) {
  if (!key) return false
  const [kind, name] = [key.slice(0, key.indexOf(':')), key.slice(key.indexOf(':') + 1)]
  return s.kind === kind && (s.name === name || s.name.replace(/^\d+\./, '') === name)
}
