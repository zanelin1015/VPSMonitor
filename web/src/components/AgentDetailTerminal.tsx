import { useEffect, useRef } from 'react'
import { Switch } from 'antd'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'

import { buildAgentTerminalURL } from '../lib/appHelpers'

interface TerminalWSMessage {
  type: string
  session_id?: string
  data?: string
  error?: string
  exit_code?: number
  rows?: number
  cols?: number
  shell?: string
}

export function defaultTerminalShell(clientOS?: string, systemVersion?: string): string {
  if (String(clientOS || '').toLowerCase().includes('windows')) {
    return 'powershell'
  }
  const version = String(systemVersion || '').toLowerCase()
  return version.includes('openwrt') || version.includes('istoreos') ? 'sh' : 'bash'
}

export function FeatureSwitch({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className={`agent-feature-switch${checked ? ' active' : ''}`}>
      <span>{label}</span>
      <Switch size="small" checked={checked} onChange={onChange} />
    </div>
  )
}

export function RemoteTTYTerminal(props: { agentID: string; shell: string; active: boolean; fontSize: number; expanded: boolean }) {
  const { agentID, shell, active, fontSize, expanded } = props
  const containerRef = useRef<HTMLDivElement | null>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const sendRef = useRef<(message: TerminalWSMessage) => void>(() => undefined)

  useEffect(() => {
    const terminal = terminalRef.current
    const fitAddon = fitAddonRef.current
    if (!active || !terminal || !fitAddon) {
      return
    }
    terminal.options.fontSize = fontSize
    window.setTimeout(() => {
      fitAddon.fit()
      sendRef.current({ type: 'resize', cols: terminal.cols, rows: terminal.rows })
      terminal.focus()
    }, 0)
  }, [active, expanded, fontSize])

  useEffect(() => {
    if (!active || !agentID || !containerRef.current) {
      return undefined
    }
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
      fontSize,
      rows: 36,
      scrollback: 5000,
      theme: {
        background: '#07111f',
        foreground: '#dbeafe',
        cursor: '#f97316',
        selectionBackground: '#334155',
      },
    })
    const fitAddon = new FitAddon()
    terminalRef.current = terminal
    fitAddonRef.current = fitAddon
    terminal.loadAddon(fitAddon)
    terminal.open(containerRef.current)
    fitAddon.fit()
    terminal.focus()
    terminal.writeln('Connecting to VPSMonitor Client realtime TTY...')

    let closed = false
    let currentSessionID = ''
    const socket = new WebSocket(buildAgentTerminalURL(agentID, shell, terminal.cols, terminal.rows))
    const send = (message: TerminalWSMessage) => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ ...message, session_id: currentSessionID || message.session_id }))
      }
    }
    sendRef.current = send
    const dataDisposable = terminal.onData((data) => send({ type: 'input', data: normalizeTerminalInput(data, shell) }))
    const resizeDisposable = terminal.onResize((size) => send({ type: 'resize', cols: size.cols, rows: size.rows }))
    const resizeWindow = () => {
      fitAddon.fit()
      send({ type: 'resize', cols: terminal.cols, rows: terminal.rows })
    }
    window.addEventListener('resize', resizeWindow)

    socket.onopen = () => {
      terminal.writeln('\r\nConnected, waiting for remote shell...')
    }
    socket.onmessage = (event) => {
      const message = JSON.parse(event.data) as TerminalWSMessage
      if (message.session_id) {
        currentSessionID = message.session_id
      }
      switch (message.type) {
        case 'terminal_opened':
          terminal.writeln(`\r\nTTY opened (${message.shell || shell}, ${message.cols || terminal.cols}x${message.rows || terminal.rows})\r\n`)
          break
        case 'terminal_output':
          terminal.write(message.data || '')
          break
        case 'terminal_error':
          terminal.writeln(`\r\n[error] ${message.error || 'unknown error'}\r\n`)
          break
        case 'terminal_closed':
          terminal.writeln(`\r\n[closed] exit=${message.exit_code ?? '-'} ${message.error || ''}\r\n`)
          break
        default:
          break
      }
    }
    socket.onerror = () => {
      terminal.writeln('\r\n[error] WebSocket connection failed\r\n')
    }
    socket.onclose = () => {
      if (!closed) {
        terminal.writeln('\r\n[disconnected]\r\n')
      }
    }

    window.setTimeout(resizeWindow, 80)
    return () => {
      closed = true
      send({ type: 'close' })
      window.removeEventListener('resize', resizeWindow)
      dataDisposable.dispose()
      resizeDisposable.dispose()
      socket.close()
      terminal.dispose()
      terminalRef.current = null
      fitAddonRef.current = null
      sendRef.current = () => undefined
    }
  }, [active, agentID, shell])

  return <div ref={containerRef} className={`remote-tty-terminal${expanded ? ' remote-tty-terminal-expanded' : ''}`} />
}

function normalizeTerminalInput(data: string, shell: string): string {
  if (data !== '\r') {
    return data
  }
  const normalizedShell = String(shell || '').toLowerCase()
  return normalizedShell === 'powershell' || normalizedShell === 'pwsh' || normalizedShell === 'cmd' ? '\r\n' : '\n'
}
