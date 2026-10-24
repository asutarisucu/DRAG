import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { CloseShell, OpenShell, ShellInput, ShellResize } from '../../wailsjs/go/main/App'
import { attachShell } from '../shellBus'

// 右下のターミナル。サーバーごとに1つシェルを開く
export function TerminalPanel({ serverId, visible }: { serverId: string; visible: boolean }) {
  const { t } = useTranslation()
  const host = useRef<HTMLDivElement>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const termRef = useRef<Terminal | null>(null)

  useEffect(() => {
    const term = new Terminal({
      fontFamily: '"Cascadia Mono", Consolas, "BIZ UDGothic", monospace',
      fontSize: 13,
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

    let shellId = ''
    let detach = () => {}
    let disposed = false
    OpenShell(serverId, term.cols, term.rows, '').then(id => {
      if (disposed) {
        CloseShell(id)
        return
      }
      shellId = id
      detach = attachShell(id, {
        out: data => term.write(data),
        exit: () => term.write(`\r\n\x1b[2m${t('terminal.ended')}\x1b[0m\r\n`),
      })
    }, err => term.write(`\x1b[31m${String(err)}\x1b[0m\r\n`))

    const onData = term.onData(d => {
      if (shellId) ShellInput(shellId, d)
    })
    const onResize = term.onResize(({ cols, rows }) => {
      if (shellId) ShellResize(shellId, cols, rows)
    })
    const ro = new ResizeObserver(() => {
      // 隠れているときは大きさが測れないので合わせない
      if (host.current && host.current.offsetWidth > 0) fit.fit()
    })
    ro.observe(host.current!)

    return () => {
      disposed = true
      ro.disconnect()
      onData.dispose()
      onResize.dispose()
      detach()
      if (shellId) CloseShell(shellId)
      term.dispose()
    }
  }, [serverId, t])

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
