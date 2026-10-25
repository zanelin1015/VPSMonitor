import type { DashboardAgentView, DashboardTagView } from '../types'
import type { AgentHealthFilter } from './agentHealth'
import type { AgentViewMode } from './appHelperTypes'

export interface AgentListProps {
  agents: DashboardAgentView[]
  loading: boolean
  error: string
  selectedTag: string
  healthFilter: AgentHealthFilter
  selectedAgentId: string
  tagFilterOptions: DashboardTagView[]
  viewMode: AgentViewMode
  panelExpanded: boolean
  topologyVisible: boolean
  restrictedView?: boolean
  onToggleViewMode: () => void
  onToggleTopology: () => void
  onRefresh: () => void
  onSelectTag: (tag: string) => void
  onHealthFilterChange: (filter: AgentHealthFilter) => void
  onSelectAgent: (agentID: string, active: boolean) => void
  searchText: string
  onSearchTextChange: (value: string) => void
}
