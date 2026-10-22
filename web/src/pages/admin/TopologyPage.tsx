import { Alert, Button, Card, Spin } from 'antd'
import { renderCNFlowPanel } from '../../components/DashboardTopologyPanels'

export function TopologyPage({
  panel,
  loading,
  loaded,
  error,
  onRetry,
}: {
  panel: Parameters<typeof renderCNFlowPanel>[0]
  loading: boolean
  loaded: boolean
  error: string
  onRetry: () => void
}) {
  return (
    <div id="topology-panel">
      {loading && !loaded ? (
        <Card>
          <Spin tip="正在计算链路拓扑...">
            <div style={{ minHeight: 160 }} />
          </Spin>
        </Card>
      ) : error && !loaded ? (
        <Alert
          type="error"
          showIcon
          message="链路拓扑加载失败"
          description={error}
          action={<Button onClick={onRetry}>重试</Button>}
        />
      ) : (
        renderCNFlowPanel(panel)
      )}
    </div>
  )
}
