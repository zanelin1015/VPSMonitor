import { Segmented, Space, Typography } from 'antd'
import type { NodeImportTarget } from '../lib/nodeImport'

export function NodeImportTargetSelector({ target, onChange }: {
  target: NodeImportTarget
  onChange: (target: NodeImportTarget) => void
}) {
  return (
    <Space wrap size={8}>
      <Typography.Text type="secondary">导入软件</Typography.Text>
      <Segmented
        aria-label="节点导入软件"
        value={target}
        options={[
          { label: 'Shadowrocket', value: 'shadowrocket' },
          { label: '其他客户端', value: 'generic' },
        ]}
        onChange={(value) => onChange(value as NodeImportTarget)}
      />
    </Space>
  )
}
