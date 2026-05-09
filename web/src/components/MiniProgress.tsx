import { clampMetricPercent } from '../lib/traffic'

export function MiniProgress(props: {
  label: string
  value: string
  percent?: number
  showTrack?: boolean
  level?: 'ok' | 'warn' | 'bad' | 'neutral'
  className?: string
}) {
  const level = props.level || 'ok'
  const hasPercent = typeof props.percent === 'number' && Number.isFinite(props.percent)
  const showTrack = hasPercent || Boolean(props.showTrack)
  const percent = clampMetricPercent(props.percent)

  return (
    <div className={`mini-progress mini-progress-${level}${props.className ? ` ${props.className}` : ''}`}>
      <div className="mini-progress-head">
        <span>{props.label}</span>
        <span>{props.value}</span>
      </div>
      {showTrack ? (
        <div className="mini-progress-track" aria-label={`${props.label} ${percent.toFixed(1)}%`}>
          <span className="mini-progress-fill" style={{ width: `${percent}%` }} />
        </div>
      ) : null}
    </div>
  )
}

