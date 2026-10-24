import type { IconName } from './components/Icon'

// ファイルの種類。開くウィンドウとアイコンを決める (mock/app.js の fileKind と同じ分け方)
export type Kind = 'dir' | 'log' | 'img' | 'arc' | 'bin' | 'code' | 'text'

export function fileKind(name: string, dir: boolean): Kind {
  if (dir) return 'dir'
  const n = name.toLowerCase()
  if (n.endsWith('.log')) return 'log'
  if (/\.(png|jpe?g|gif|webp|svg|bmp|ico)$/.test(n)) return 'img'
  if (/\.(gz|tgz|tar|zip|jar|7z)$/.test(n)) return 'arc'
  if (/\.(mca|dat|class|ibd|bin|so|exe|o)$/.test(n) || /^ibdata\d|^aria_log/.test(n)) return 'bin'
  if (/\.(sh|json|ya?ml|toml|properties|js|ts|py|env|conf|ini)$/.test(n)) return 'code'
  return 'text'
}

export const KIND_ICON: Record<Kind, [IconName, string]> = {
  dir: ['folder', 'f-dir'], log: ['log', 'f-log'], img: ['image', 'f-img'], arc: ['archive', 'f-arc'],
  bin: ['file', 'f-arc'], code: ['code', 'f-code'], text: ['file', 'f-file'],
}

// プロパティに出す種類の名前の翻訳キー
export const KIND_LABEL: Record<Kind, string> = {
  dir: 'props.folder', log: 'props.log', img: 'props.image', arc: 'props.archive', bin: 'props.binary', code: 'props.code', text: 'props.file',
}

// ログの行を、先頭の [時刻 レベル]: と本文に分け、色の種類を決める (mock/ の logParts と同じ)
export function logParts(line: string) {
  const m = line.match(/^(\[[^\]]*\]:?\s?)(.*)$/)
  const head = m ? m[1] : '', body = m ? m[2] : line
  const c = /\bWARN\b/.test(head) || /\[Warning\]|" [45]\d\d /.test(body) ? 'warn'
    : /\bERROR\b/.test(head) || /\[ERROR\]/.test(body) ? 'err'
    : /joined the game|Done \(|backup done/.test(body) ? 'ok' : ''
  return { head, body, c }
}
