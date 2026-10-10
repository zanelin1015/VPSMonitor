import { Button } from 'antd'
import { MoonOutlined, SunOutlined, SyncOutlined } from '@ant-design/icons'
import { useAppTheme } from '../theme'
import { customerThemeLabels, nextCustomerTheme } from '../lib/customerTheme'

export function CustomerThemeButton() {
  const { mode, effectiveMode, setMode } = useAppTheme()
  const nextMode = nextCustomerTheme(mode)
  const label = `当前主题：${customerThemeLabels[mode]}${mode === 'system' ? `（${customerThemeLabels[effectiveMode]}）` : ''}；点击切换为${customerThemeLabels[nextMode]}`
  return <Button
    shape="circle"
    className="customer-theme-button"
    aria-label={label}
    title={label}
    icon={mode === 'system' ? <SyncOutlined /> : mode === 'dark' ? <MoonOutlined /> : <SunOutlined />}
    onClick={() => setMode(nextMode)}
  />
}
