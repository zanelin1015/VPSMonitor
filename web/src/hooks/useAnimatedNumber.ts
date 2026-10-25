import { useEffect, useRef, useState } from 'react'

export function useAnimatedNumber(target: number, duration = 950): number {
  const safeTarget = Number.isFinite(target) ? Math.max(0, target) : 0
  const [value, setValue] = useState(safeTarget)
  const currentRef = useRef(safeTarget)

  useEffect(() => {
    const from = currentRef.current
    const delta = safeTarget - from
    if (Math.abs(delta) < 0.5) {
      currentRef.current = safeTarget
      setValue(safeTarget)
      return undefined
    }

    let frame = 0
    const startedAt = performance.now()
    const tick = (now: number) => {
      const progress = Math.min(1, Math.max(0, (now - startedAt) / duration))
      const eased = 1 - Math.pow(1 - progress, 3)
      const next = from + delta * eased
      currentRef.current = next
      setValue(next)
      if (progress < 1) {
        frame = requestAnimationFrame(tick)
      } else {
        currentRef.current = safeTarget
        setValue(safeTarget)
      }
    }

    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
    }
  }, [duration, safeTarget])

  return value
}
