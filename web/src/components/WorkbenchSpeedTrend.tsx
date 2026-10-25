import { useEffect, useState } from 'react'
import { formatSpeed } from '../lib/traffic'
import { createInitialSpeedHistory, speedAxisInfo, speedBucketStart, SPEED_HISTORY_POINTS, type SpeedHistoryPoint } from '../lib/speedHistory'
import { useAnimatedNumber } from '../hooks/useAnimatedNumber'
import { SpeedHistoryChart } from './SpeedHistoryChart'

export function WorkbenchSpeedTrend(props: { up: number; down: number }) {
  const { up, down } = props
  const animatedUp = useAnimatedNumber(up)
  const animatedDown = useAnimatedNumber(down)
  const [history, setHistory] = useState<SpeedHistoryPoint[]>(() => createInitialSpeedHistory(up, down))
  const animatedTotal = animatedUp + animatedDown
  const chartRows = history.map((point, index) => (
    index === history.length - 1 && point.time === speedBucketStart(Date.now())
      ? { ...point, up: animatedUp, down: animatedDown }
      : point
  ))
  const axis = speedAxisInfo(chartRows)
  useEffect(() => {
    setHistory((current) => {
      const now = Date.now()
      const bucket = speedBucketStart(now)
      const safeUp = Number.isFinite(up) ? Math.max(0, up) : 0
      const safeDown = Number.isFinite(down) ? Math.max(0, down) : 0
      const last = current[current.length - 1]
      if (last?.time === bucket) {
        const samples = Math.max(1, last.samples || 1)
        const nextSamples = samples + 1
        return [
          ...current.slice(0, -1),
          {
            time: bucket,
            samples: nextSamples,
            up: ((last.up * samples) + safeUp) / nextSamples,
            down: ((last.down * samples) + safeDown) / nextSamples,
          },
        ]
      }
      return [...current, { up: safeUp, down: safeDown, time: bucket, samples: 1 }].slice(-SPEED_HISTORY_POINTS)
    })
  }, [down, up])

  return (
    <div className="workbench-speed-trend">
      <div className="workbench-speed-current">
        <div className="workbench-speed-current-total">
          <span>当前总网速</span>
          <strong>{formatSpeed(animatedTotal)}</strong>
        </div>
        <div className="workbench-speed-legend">
          <span className="workbench-speed-upload-pill"><i className="legend-dot legend-upload" />上传 {formatSpeed(animatedUp)}</span>
          <span className="workbench-speed-download-pill"><i className="legend-dot legend-download" />下载 {formatSpeed(animatedDown)}</span>
        </div>
      </div>
      <div className="workbench-speed-chart" aria-label={`实时网速，上传 ${formatSpeed(animatedUp)}，下载 ${formatSpeed(animatedDown)}`}>
        <SpeedHistoryChart rows={chartRows} axis={axis} />
      </div>
      <div className="workbench-speed-scale">
        <span>最近 {SPEED_HISTORY_POINTS} 个 5 分钟均值</span>
        <span>纵轴单位 {axis.unit}/s · 峰值 {formatSpeed(axis.maxValue)}</span>
      </div>
    </div>
  )
}
