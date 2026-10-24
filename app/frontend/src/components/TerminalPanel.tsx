import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { CloseShell, OpenShell, ShellInput, ShellResize } from '../../wailsjs/go/main/App'
import { attachShell } from '../shellBus'

// 右下のターミナル。サーバーごとに1つシェルを開く。
// generation は再接続のたびに増える。増えたら同じ画面のまま新しいシェルを開き直す
export function TerminalPanel({ serverId, generation, visible, fontSize }: {
  serverId: string
  generation: number
  visible: boolean
  fontSize: number
}) {
  const { t } = useTranslation()
  const host = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const shellRef = useRef('')
  // 言語を切り替えてもシェルを開き直さないよう、文言は ref から読む
  const tRef = useRef(t)
  tRef.current = t

  useEffect(() => {
    const term = new Terminal({
      fontFamily: '"Cascadia Mono", Consolas, "BIZ UDGothic", monospace',
      fontSize,
      lineHeight: 1.15,
      cursorBlink: true,
      scrollback: 5000,
      theme: { background: '#0a0d11', foreground: '#cfd6e0', cursor: '#5b9dff', selectionBackground: '#5b9dff55' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host.current!)
    fit.fit()
    termRef.current = term
    fitRef.current = fit
    const onData = term.onData(d => { if (shellRef.current) ShellInput(shellRef.current, d) })
    const onResize = term.onResize(({ cols, rows }) => { if (shellRef.current) ShellResize(shellRef.current, cols, rows) })
    const ro = new ResizeObserver(() => {
      // 隠れているときは大きさが測れないので合わせない
      if (host.current && host.current.offsetWidth > 0) fit.fit()
    })
    ro.observe(host.current!)
    return () => {
      ro.disconnect()
      onData.dispose()
      onResize.dispose()
      term.dispose()
      termRef.current = null
    }
    // 文字の大きさは下の effect で変える。ここで作り直すと画面が消える
  }, [])

  useEffect(() => {
    const term = termRef.current!
    if (generation > 0) term.write(`\r\n\x1b[2m${tRef.current('terminal.reopened')}\x1b[0m\r\n`)
    let detach = () => {}
    let disposed = false
    OpenShell(serverId, term.cols, term.rows, '').then(id => {
      if (disposed) {
        CloseShell(id)
        return
      }
      shellRef.current = id
      detach = attachShell(id, {
        out: data => term.write(data),
        exit: () => {
          if (shellRef.current === id) shellRef.current = ''
          term.write(`\r\n\x1b[2m${tRef.current('terminal.ended')}\x1b[0m\r\n`)
        },
      })
    }, err => term.write(`\x1b[31m${String(err)}\x1b[0m\r\n`))
    return () => {
      disposed = true
      detach()
      if (shellRef.current) CloseShell(shellRef.current)
      shellRef.current = ''
    }
  }, [serverId, generation])

  useEffect(() => {
    const term = termRef.current
    if (!term || term.options.fontSize === fontSize) return
    term.options.fontSize = fontSize
    fitRef.current?.fit()
  }, [fontSize])

  useEffect(() => {
    if (visible) {
      fitRef.current?.fit()
      termRef.current?.focus()
    }
  }, [visible])

  return (
    <section className="flex min-h-0 flex-col border-t border-line bg-term">
      <div className="flex h-[34px] flex-none items-center gap-2.5 border-b border-line bg-panel pr-2 pl-3.5">
        <span className="text-[11px] font-semibold tracking-[.08em] text-muted">{t('terminal.title')}</span>
      </div>
      <div ref={host} className="min-h-0 flex-1" />
    </section>
  )
}
