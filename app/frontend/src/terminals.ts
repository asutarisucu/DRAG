// ファイルツリーやデスクトップから、右下のターミナルを操作するための窓口。サーバーごとに1つ
export type TerminalApi = {
  cd: (path: string) => void
  insert: (text: string) => void
  // 前面のコマンドをデスクトップのウィンドウに移す。pos はデスクトップの中の位置
  popOut: (pos?: { x: number; y: number }) => void
  cwd: () => Promise<string>
  // デスクトップのウィンドウのセッションをターミナルに戻す
  takeBack: (winId: string) => void
  // ウィンドウをドラッグしている間、ターミナルに「ここで離すと…」を出す
  setDropLabel: (label: string) => void
  // 前面のコマンド。シェル自身なら空
  foreground: () => string
}

const terminals = new Map<string, TerminalApi>()

export function registerTerminal(serverId: string, api: TerminalApi) {
  terminals.set(serverId, api)
  return () => {
    if (terminals.get(serverId) === api) terminals.delete(serverId)
  }
}

export function terminalOf(serverId: string): TerminalApi | undefined {
  return terminals.get(serverId)
}

// ホームの下は ~ で書き、空白や記号を含むときは単一引用符で囲む (mock/ の pathArg と同じ)
export function pathArg(home: string, p: string) {
  const t = p === home ? '~' : p.startsWith(home + '/') ? '~' + p.slice(home.length) : p
  return /[\s'"$`\\!&|;<>()*?#]/.test(t) ? `'${p.replace(/'/g, `'\\''`)}'` : t
}
