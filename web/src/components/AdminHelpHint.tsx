import type { ReactNode } from 'react'
import { Tooltip } from 'antd'
import { QuestionCircleOutlined } from '@ant-design/icons'

export function AdminHelpHint({ title }: { title: ReactNode }) {
  return (
    <Tooltip trigger={['click']} title={title}>
      <QuestionCircleOutlined className="admin-help-icon" aria-label="查看说明" />
    </Tooltip>
  )
}
