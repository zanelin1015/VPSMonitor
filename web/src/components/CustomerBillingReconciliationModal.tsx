import { Alert, Button, Empty, Modal, Space, Table } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { CustomerBillingDiagnostic, CustomerBillingReconciliationResponse } from '../types'
import { listPagination } from '../lib/pagination'

interface CustomerBillingReconciliationModalProps {
  open: boolean
  onClose: () => void
  result: CustomerBillingReconciliationResponse | null
  loading: boolean
  columns: ColumnsType<CustomerBillingDiagnostic>
}

export function CustomerBillingReconciliationModal({ open, onClose, result, loading, columns }: CustomerBillingReconciliationModalProps) {
  return (
    <Modal
      title="用户费用与客户端绑定对账"
      open={open}
      onCancel={onClose}
      footer={<Button onClick={onClose}>关闭</Button>}
      width={1080}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message={result ? `已匹配 ${result.matched}，旧数据 ${result.legacy}，需修复 ${result.mismatch}，未找到 ${result.missing}` : '按最新 x-ui 上报检查授权绑定'}
          description="“重新绑定”会保留授权名称、费用模式和前置代理，只把客户端关联更新为当前 x-ui 的稳定 ID。"
        />
        <Table
          rowKey={(record) => record.assignment_id}
          loading={loading}
          columns={columns}
          dataSource={result?.items || []}
          pagination={listPagination}
          scroll={{ x: 940 }}
          locale={{ emptyText: <Empty description="暂无可对账的授权" /> }}
        />
      </Space>
    </Modal>
  )
}
