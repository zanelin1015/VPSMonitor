import { useEffect, useRef, useState } from 'react'
import { formatChartAxisValue, formatChartTime, type SpeedAxisInfo, type SpeedHistoryPoint } from '../lib/speedHistory'
import { formatSpeed } from '../lib/traffic'

/** Two responsive SVG series; no chart runtime or external rendering dependency. */
export function SpeedHistoryChart({ rows, axis }: { rows: SpeedHistoryPoint[]; axis: SpeedAxisInfo }) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [size, setSize] = useState({ width: 640, height: 220 })
  const [activeIndex, setActiveIndex] = useState<number | null>(null)

  useEffect(() => {
    const element = containerRef.current
    if (!element) return
    const measure = () => {
      const { width, height } = element.getBoundingClientRect()
      if (width > 0 && height > 0) setSize({ width, height })
    }
    measure()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure)
      return () => window.removeEventListener('resize', measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const left = size.width < 400 ? 58 : 70
  const right = size.width - 18
  const top = 18
  const bottom = Math.max(top + 40, size.height - 30)
  const x = (index: number) => left + index / Math.max(1, rows.length - 1) * Math.max(1, right - left)
  const y = (speed: number) => bottom - Math.min(1, Math.max(0, Number.isFinite(speed) ? speed / axis.maxValue : 0)) * (bottom - top)
  const path = (key: 'up' | 'down') => rows.map((row, index) => `${index ? 'L' : 'M'}${x(index)},${y(row[key])}`).join(' ')
  const active = activeIndex === null ? null : rows[Math.min(activeIndex, rows.length - 1)]
  const tickIndices = [...new Set([0, Math.floor((rows.length - 1) / 2), rows.length - 1])].filter(index => index >= 0)

  return (
    <div ref={containerRef} className="speed-history-plot">
      <svg viewBox={`0 0 ${size.width} ${size.height}`} role="img" aria-label="上传与下载网速趋势，左右方向键查看采样点" tabIndex={0}
        onPointerMove={(event) => {
          const bounds = event.currentTarget.getBoundingClientRect()
          if (bounds.width <= 0 || !rows.length) return
          const cursor = (event.clientX - bounds.left) / bounds.width * size.width
          setActiveIndex(Math.max(0, Math.min(rows.length - 1, Math.round((cursor - left) / Math.max(1, right - left) * (rows.length - 1)))))
        }}
        onPointerLeave={() => setActiveIndex(null)} onBlur={() => setActiveIndex(null)}
        onKeyDown={(event) => {
          if (!rows.length) return
          if (['ArrowLeft', 'ArrowRight', 'Home', 'End', 'Escape'].includes(event.key)) event.preventDefault()
          if (event.key === 'Escape') setActiveIndex(null)
          if (event.key === 'Home') setActiveIndex(0)
          if (event.key === 'End') setActiveIndex(rows.length - 1)
          if (event.key === 'ArrowLeft') setActiveIndex(index => Math.max(0, (index ?? rows.length) - 1))
          if (event.key === 'ArrowRight') setActiveIndex(index => Math.min(rows.length - 1, (index ?? -1) + 1))
        }}>
        <title>上传与下载网速</title>
        {[0, 1, 2, 3].map(index => {
          const value = axis.maxValue * index / 3
          return <g key={index}><line className="speed-history-grid" x1={left} x2={right} y1={y(value)} y2={y(value)} /><text x={left - 8} y={y(value) + 4} textAnchor="end">{formatChartAxisValue(value / axis.divisor, axis.unit)}</text></g>
        })}
        {tickIndices.map(index => <text key={index} x={x(index)} y={size.height - 9} textAnchor={index === 0 ? 'start' : index === rows.length - 1 ? 'end' : 'middle'}>{formatChartTime(rows[index].time)}</text>)}
        <path className="speed-history-series speed-history-upload" d={path('up')} />
        <path className="speed-history-series speed-history-download" d={path('down')} />
        {active && activeIndex !== null ? <g>
          <line className="speed-history-crosshair" x1={x(activeIndex)} x2={x(activeIndex)} y1={top} y2={bottom} />
          <circle className="speed-history-upload" cx={x(activeIndex)} cy={y(active.up)} r={3} />
          <circle className="speed-history-download" cx={x(activeIndex)} cy={y(active.down)} r={3} />
        </g> : null}
      </svg>
      {active ? <div className="speed-history-tooltip" role="status" aria-live="polite" style={{ left: activeIndex !== null && activeIndex < rows.length / 2 ? 'auto' : 12, right: activeIndex !== null && activeIndex < rows.length / 2 ? 12 : 'auto' }}>
        <time>{formatChartTime(active.time)}</time><span>上传 {formatSpeed(active.up)}</span><span>下载 {formatSpeed(active.down)}</span>
      </div> : null}
    </div>
  )
}
