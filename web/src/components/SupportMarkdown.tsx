import { Fragment, type ReactNode } from 'react'

const allowedHosts = new Set(['t.me', 'wa.me', 'api.whatsapp.com'])

export function SupportMarkdown({ body, className = '' }: { body: string; className?: string }) {
  const lines = String(body || '').replace(/\r\n?/g, '\n').split('\n')
  const blocks: ReactNode[] = []
  let list: ReactNode[] = []

  const flushList = () => {
    if (!list.length) return
    blocks.push(<ul key={`list-${blocks.length}`}>{list}</ul>)
    list = []
  }

  lines.forEach((line, index) => {
    const trimmed = line.trim()
    if (trimmed.startsWith('- ')) {
      list.push(<li key={`item-${index}`}>{renderInline(trimmed.slice(2))}</li>)
      return
    }
    flushList()
    blocks.push(<div key={`line-${index}`}>{renderInline(line) || '\u00a0'}</div>)
  })
  flushList()

  return <div className={`support-markdown${className ? ` ${className}` : ''}`}>{blocks}</div>
}

function renderInline(value: string): ReactNode[] {
  const token = /(\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)|\*\*([^*]+)\*\*|(https:\/\/(?:t\.me|wa\.me|api\.whatsapp\.com)\/[^\s<]+))/g
  const output: ReactNode[] = []
  let lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = token.exec(value)) !== null) {
    if (match.index > lastIndex) output.push(value.slice(lastIndex, match.index))
    if (match[2] && isAllowedLink(match[3])) {
      output.push(<a key={`link-${match.index}`} href={match[3]} target="_blank" rel="noopener noreferrer">{match[2]}</a>)
    } else if (match[4]) {
      output.push(<strong key={`strong-${match.index}`}>{match[4]}</strong>)
    } else if (match[5] && isAllowedLink(match[5])) {
      output.push(<a key={`url-${match.index}`} href={match[5]} target="_blank" rel="noopener noreferrer">{match[5]}</a>)
    } else {
      output.push(<Fragment key={`text-${match.index}`}>{match[0]}</Fragment>)
    }
    lastIndex = token.lastIndex
  }
  if (lastIndex < value.length) output.push(value.slice(lastIndex))
  return output
}

function isAllowedLink(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'https:' && allowedHosts.has(url.hostname.toLowerCase())
  } catch {
    return false
  }
}
