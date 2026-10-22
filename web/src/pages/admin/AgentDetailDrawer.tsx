import { Drawer, Spin } from 'antd'
import { lazy, Suspense } from 'react'
import type { AgentDetailPanelProps } from '../../components/AgentDetailPanel'

const AgentDetailPanel = lazy(() =>
  import('../../components/AgentDetailPanel').then((module) => ({ default: module.AgentDetailPanel })),
)

export function AgentDetailDrawer({ detail, onClose }: { detail: AgentDetailPanelProps | null; onClose: () => void }) {
  return (
    <Drawer
      className="admin-agent-drawer"
      title={detail ? `${detail.selectedAgent.agent_name || detail.selectedAgentId} · Client 详情` : 'Client 详情'}
      open={Boolean(detail)}
      zIndex={900}
      width="min(1440px, 96vw)"
      onClose={onClose}
      destroyOnClose
    >
      {detail ? (
        <Suspense
          fallback={
            <Spin tip="正在加载 Client 详情">
              <div style={{ minHeight: 180 }} />
            </Spin>
          }
        >
          <AgentDetailPanel key={detail.selectedAgentId} {...detail} onBackToList={onClose} />
        </Suspense>
      ) : null}
    </Drawer>
  )
}
