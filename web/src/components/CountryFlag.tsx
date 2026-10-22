import { useState } from 'react'
import { GlobalOutlined } from '@ant-design/icons'

// Bundle the images with the app: flag emoji depend on the OS font and locale.
const flagImages = import.meta.glob<string>('../assets/flags/*.svg', {
  eager: true,
  import: 'default',
  query: '?url',
})

export function CountryFlag({ code, label }: { code?: string; label?: string }) {
  const normalized = (code || '').trim().toUpperCase()
  const validCode = /^[A-Z]{2}$/.test(normalized) ? normalized : ''
  const source = flagImages[`../assets/flags/${validCode.toLowerCase()}.svg`]
  const [failedSource, setFailedSource] = useState('')
  const accessibleLabel = label || validCode || '未知地区'

  return (
    <span className="country-flag" role="img" aria-label={accessibleLabel} title={accessibleLabel}>
      {source && failedSource !== source ? (
        <img src={source} alt="" aria-hidden="true" draggable={false} onError={() => setFailedSource(source)} />
      ) : validCode ? (
        <span className="country-flag-code" aria-hidden="true">{validCode}</span>
      ) : (
        <GlobalOutlined aria-hidden="true" />
      )}
    </span>
  )
}
