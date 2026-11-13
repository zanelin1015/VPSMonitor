import { useEffect, useState } from 'react'

const compactLayoutQuery = '(max-width: 980px), (max-width: 1366px) and (pointer: coarse)'

export function useCompactLayout() {
  const [compact, setCompact] = useState(() => typeof window !== 'undefined' && Boolean(window.matchMedia?.(compactLayoutQuery).matches))

  useEffect(() => {
    const media = window.matchMedia?.(compactLayoutQuery)
    if (!media) return
    const update = () => setCompact(media.matches)
    update()
    media.addEventListener?.('change', update)
    return () => media.removeEventListener?.('change', update)
  }, [])

  return compact
}
