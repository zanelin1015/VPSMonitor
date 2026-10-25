export type SpeedHistoryPoint = {
  up: number
  down: number
  time: number
  samples: number
}

export type SpeedAxisInfo = {
  maxValue: number
  unit: string
  divisor: number
}

export const SPEED_BUCKET_MS = 5 * 60 * 1000
export const SPEED_HISTORY_POINTS = 24

export function createInitialSpeedHistory(up: number, down: number): SpeedHistoryPoint[] {
  const currentBucket = speedBucketStart(Date.now())
  const safeUp = Number.isFinite(up) ? Math.max(0, up) : 0
  const safeDown = Number.isFinite(down) ? Math.max(0, down) : 0
  return Array.from({ length: SPEED_HISTORY_POINTS }, (_, index) => ({
    up: safeUp,
    down: safeDown,
    samples: 1,
    time: currentBucket - (SPEED_HISTORY_POINTS - 1 - index) * SPEED_BUCKET_MS,
  }))
}

export function speedBucketStart(value: number): number {
  return Math.floor(value / SPEED_BUCKET_MS) * SPEED_BUCKET_MS
}

export function speedAxisInfo(rows: SpeedHistoryPoint[]): SpeedAxisInfo {
  const rawMax = Math.max(1, ...rows.flatMap((row) => [row.up, row.down]))
  const unitInfo = speedUnitInfo(rawMax)
  const niceMax = niceAxisMax(rawMax / unitInfo.divisor) * unitInfo.divisor
  return {
    maxValue: niceMax,
    unit: unitInfo.unit,
    divisor: unitInfo.divisor,
  }
}

function speedUnitInfo(maxValue: number): { unit: string; divisor: number } {
  if (maxValue >= 1024 * 1024 * 1024) {
    return { unit: 'GB', divisor: 1024 * 1024 * 1024 }
  }
  if (maxValue >= 1024 * 1024) {
    return { unit: 'MB', divisor: 1024 * 1024 }
  }
  if (maxValue >= 1024) {
    return { unit: 'KB', divisor: 1024 }
  }
  return { unit: 'B', divisor: 1 }
}

function niceAxisMax(value: number): number {
  if (value <= 0) {
    return 1
  }
  const exponent = Math.floor(Math.log10(value))
  const base = 10 ** exponent
  const normalized = value / base
  const nice = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return nice * base
}

export function formatChartAxisValue(value: number, unit: string): string {
  const safeValue = Number.isFinite(value) ? value : 0
  if (safeValue === 0) {
    return `0 ${unit}/s`
  }
  return `${safeValue >= 10 ? safeValue.toFixed(0) : safeValue.toFixed(1)} ${unit}/s`
}

export function formatChartTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(value))
}

