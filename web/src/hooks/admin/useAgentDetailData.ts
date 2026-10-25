import type { MessageInstance } from 'antd/es/message/interface'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import { useEffect, useRef, useState } from 'react'
import { useCurrentAgentRequest } from '../../hooks/useCurrentAgentRequest'
import type { ConfigSectionKey } from '../../lib/appHelpers'
import {
  buildSectionSavePayload,
  configSectionLabel,
  configSignature,
  createEmptyManagedConfig,
  fetchJSON,
  formatAddressInput,
  isUnauthorized,
  mergeSavedSectionIntoDraft,
  mergeTagOptions,
  normalizeManagedConfig,
  normalizeXUIOverview,
  parseTagInput,
} from '../../lib/appHelpers'
import type {
  AdminUser,
  AgentLogsResponse,
  AreaAgentTagsResponse,
  ConfigAuditLog,
  DashboardAgentView,
  ManagedAgentConfig,
  TagSettingsResponse,
  XUIAction,
  XUIOverview,
} from '../../types'

interface LoadOptions {
  silent?: boolean
}

function hasCustomerDisplayNameField(value: unknown): boolean {
  return Boolean(
    value && typeof value === 'object' && Object.prototype.hasOwnProperty.call(value, 'customer_display_name'),
  )
}

interface Options {
  selectedAgentId: string
  adminUser: AdminUser | null
  pendingClientSearchRef: MutableRefObject<{ agentID: string; query: string } | null>
  setActiveTabKey: Dispatch<SetStateAction<string>>
  setClientSearch: Dispatch<SetStateAction<string>>
  setSelectedOutboundTag: Dispatch<SetStateAction<string>>
  setSelectedRuleIndex: Dispatch<SetStateAction<number | null>>
  setSelectedNodeAnchor: Dispatch<SetStateAction<string>>
  reloadToken: number
  inFlightRequestsRef: MutableRefObject<Set<string>>
  setAdminUser: Dispatch<SetStateAction<AdminUser | null>>
  cachedCustomerDisplayName: (agentID: string) => string | undefined
  agents: DashboardAgentView[]
  rememberCustomerDisplayName: (agentID: string, name: string) => void
  message: MessageInstance
  setTagOptions: Dispatch<SetStateAction<string[]>>
  loadAgents: (options?: LoadOptions) => Promise<void>
  canManageSystem: boolean
  tagOptions: string[]
  selectedAgent: DashboardAgentView | undefined
}

export function useAgentDetailData({
  selectedAgentId,
  adminUser,
  pendingClientSearchRef,
  setActiveTabKey,
  setClientSearch,
  setSelectedOutboundTag,
  setSelectedRuleIndex,
  setSelectedNodeAnchor,
  reloadToken,
  inFlightRequestsRef,
  setAdminUser,
  cachedCustomerDisplayName,
  agents,
  rememberCustomerDisplayName,
  message,
  setTagOptions,
  loadAgents,
  canManageSystem,
  tagOptions,
  selectedAgent,
}: Options) {
  const [overview, setOverview] = useState<XUIOverview | null>(null)

  const [overviewLoading, setOverviewLoading] = useState(false)

  const [overviewError, setOverviewError] = useState('')

  const [managedConfig, setManagedConfig] = useState<ManagedAgentConfig | null>(null)

  const [savedManagedConfig, setSavedManagedConfig] = useState<ManagedAgentConfig | null>(null)

  const managedConfigDirtyRef = useRef(false)

  const [newTagName, setNewTagName] = useState('')

  const [tagSaving, setTagSaving] = useState(false)

  const [entryAddressInputText, setEntryAddressInputText] = useState('')

  const [configLoading, setConfigLoading] = useState(false)

  const [configSavingSection, setConfigSavingSection] = useState<ConfigSectionKey | null>(null)

  const [configError, setConfigError] = useState('')

  const [configAudits, setConfigAudits] = useState<ConfigAuditLog[]>([])

  const [configAuditsLoading, setConfigAuditsLoading] = useState(false)

  const [xuiActions, setXUIActions] = useState<XUIAction[]>([])

  const [xuiActionsLoading, setXUIActionsLoading] = useState(false)

  const [agentLogs, setAgentLogs] = useState<AgentLogsResponse | null>(null)

  const [agentLogsLoading, setAgentLogsLoading] = useState(false)

  const [agentLogsError, setAgentLogsError] = useState('')

  const lastDetailAgentIdRef = useRef('')

  const isCurrentAgentRequest = useCurrentAgentRequest(selectedAgentId)

  useEffect(() => {
    if (!adminUser) {
      return
    }
    if (!selectedAgentId) {
      lastDetailAgentIdRef.current = ''
      pendingClientSearchRef.current = null
      setActiveTabKey((current) => (current === 'config' ? 'overview' : current))
      setOverview(null)
      setManagedConfig(null)
      setSavedManagedConfig(null)
      managedConfigDirtyRef.current = false
      setEntryAddressInputText('')
      setOverviewError('')
      setConfigError('')
      setXUIActions([])
      setAgentLogs(null)
      setAgentLogsError('')
      setConfigAudits([])
      return
    }

    if (lastDetailAgentIdRef.current !== selectedAgentId) {
      lastDetailAgentIdRef.current = selectedAgentId
      setOverview(null)
      setManagedConfig(null)
      setSavedManagedConfig(null)
      managedConfigDirtyRef.current = false
      setEntryAddressInputText('')
      setOverviewError('')
      setConfigError('')
      setXUIActions([])
      setAgentLogs(null)
      setAgentLogsError('')
      setConfigAudits([])
      const pendingSearch = pendingClientSearchRef.current
      setClientSearch(pendingSearch?.agentID === selectedAgentId ? pendingSearch.query : '')
      pendingClientSearchRef.current = null
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
      setSelectedNodeAnchor('')
    }

    void loadOverview(selectedAgentId)
    void loadManagedConfig(selectedAgentId)
    void loadXUIActions(selectedAgentId)
    void loadAgentLogs(selectedAgentId)
    void loadConfigAudits(selectedAgentId)
  }, [selectedAgentId, reloadToken, adminUser])

  async function loadOverview(agentID: string, options: LoadOptions = {}) {
    const requestKey = `overview:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setOverviewLoading(true)
      setOverviewError('')
    }
    try {
      const data = await fetchJSON<XUIOverview>(`/api/v1/agents/${agentID}/xui/overview`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setOverview(normalizeXUIOverview(data))
      setOverviewError('')
      setSelectedOutboundTag('')
      setSelectedRuleIndex(null)
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        setOverview(null)
        setOverviewError(error instanceof Error ? error.message : '加载 x-ui 概览失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setOverviewLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadManagedConfig(agentID: string, options: LoadOptions = {}) {
    const requestKey = `config:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setConfigLoading(true)
      setConfigError('')
    }
    try {
      const data = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${agentID}/config`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      const cachedName = cachedCustomerDisplayName(agentID)
      const dataWithCustomerName =
        !hasCustomerDisplayNameField(data) && cachedName !== undefined
          ? { ...data, customer_display_name: cachedName }
          : data
      const agentName = agents.find((item) => item.agent_id === agentID)?.agent_name
      const normalized = normalizeManagedConfig(dataWithCustomerName, agentID, agentName)
      rememberCustomerDisplayName(agentID, normalized.customer_display_name || '')
      setSavedManagedConfig(normalized)
      if (silent && managedConfigDirtyRef.current) {
        setConfigError('')
        return
      }
      managedConfigDirtyRef.current = false
      setManagedConfig(normalized)
      setEntryAddressInputText(formatAddressInput(normalized.entry?.addresses))
      setConfigError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        const agentName = agents.find((item) => item.agent_id === agentID)?.agent_name
        const emptyConfig = createEmptyManagedConfig(agentID, agentName)
        managedConfigDirtyRef.current = false
        setSavedManagedConfig(emptyConfig)
        setManagedConfig(emptyConfig)
        setEntryAddressInputText(formatAddressInput(emptyConfig.entry?.addresses))
        setConfigError(error instanceof Error ? error.message : '加载托管配置失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setConfigLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  const actionRequestsRef = useRef(new Map<string, Promise<XUIAction[] | undefined>>())
  async function loadXUIActions(agentID = selectedAgentId, options: LoadOptions = {}): Promise<XUIAction[] | undefined> {
    if (!agentID) {
      setXUIActions([])
      return
    }
    const existing = actionRequestsRef.current.get(agentID)
    if (existing) return existing
    const silent = Boolean(options.silent)
    if (!silent) setXUIActionsLoading(true)
    const request = (async () => {
      try {
        const data = await fetchJSON<XUIAction[]>(`/api/v1/agents/${agentID}/xui/actions?limit=100`)
        const actions = Array.isArray(data) ? data : []
        if (isCurrentAgentRequest(agentID)) setXUIActions(actions)
        return actions
      } catch (error) {
        if (isUnauthorized(error)) setAdminUser(null)
        else if (!silent && isCurrentAgentRequest(agentID)) message.error(error instanceof Error ? error.message : '加载 x-ui 操作记录失败')
      } finally {
        if (!silent && isCurrentAgentRequest(agentID)) setXUIActionsLoading(false)
        actionRequestsRef.current.delete(agentID)
      }
    })()
    actionRequestsRef.current.set(agentID, request)
    return request
  }

  async function loadAgentLogs(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setAgentLogs(null)
      setAgentLogsError('')
      return
    }
    const requestKey = `logs:${agentID}`
    if (inFlightRequestsRef.current.has(requestKey)) {
      return
    }
    inFlightRequestsRef.current.add(requestKey)
    const silent = Boolean(options.silent)
    if (!silent) {
      setAgentLogsLoading(true)
      setAgentLogsError('')
    }
    try {
      const data = await fetchJSON<AgentLogsResponse>(`/api/v1/agents/${agentID}/logs`)
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setAgentLogs({ ...data, logs: Array.isArray(data.logs) ? data.logs : [] })
      setAgentLogsError('')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        setAgentLogs(null)
        setAgentLogsError(error instanceof Error ? error.message : '加载日志失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setAgentLogsLoading(false)
      }
      inFlightRequestsRef.current.delete(requestKey)
    }
  }

  async function loadConfigAudits(agentID = selectedAgentId, options: LoadOptions = {}) {
    if (!agentID) {
      setConfigAudits([])
      return
    }
    const silent = Boolean(options.silent)
    if (!silent) {
      setConfigAuditsLoading(true)
    }
    try {
      const data = await fetchJSON<ConfigAuditLog[]>(
        `/api/v1/admin/audit?agent_id=${encodeURIComponent(agentID)}&limit=8`,
      )
      if (!isCurrentAgentRequest(agentID)) {
        return
      }
      setConfigAudits(Array.isArray(data) ? data : [])
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      } else if (!silent && isCurrentAgentRequest(agentID)) {
        message.error(error instanceof Error ? error.message : '加载配置修改记录失败')
      }
    } finally {
      if (!silent && isCurrentAgentRequest(agentID)) {
        setConfigAuditsLoading(false)
      }
    }
  }

  async function loadTagSettings() {
    try {
      const data = await fetchJSON<TagSettingsResponse>('/api/v1/admin/tags')
      setTagOptions((current) => mergeTagOptions(current, data.tags || []))
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
    }
  }

  async function saveTagOptions(nextTags: string[]) {
    setTagSaving(true)
    try {
      const normalized = mergeTagOptions([], nextTags)
      const data = await fetchJSON<TagSettingsResponse>('/api/v1/admin/tags', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tags: normalized }),
      })
      setTagOptions(mergeTagOptions(normalized, data.tags || []))
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存标签失败')
      return false
    } finally {
      setTagSaving(false)
    }
  }

  async function saveAreaAgentTags(nextTags = managedConfig?.tags || []) {
    if (!selectedAgentId) {
      return false
    }
    setTagSaving(true)
    try {
      const normalized = mergeTagOptions([], nextTags)
      const data = await fetchJSON<AreaAgentTagsResponse>(
        `/api/v1/admin/area-agent-tags/${encodeURIComponent(selectedAgentId)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ tags: normalized }),
        },
      )
      const savedTags = mergeTagOptions([], data.tags || [])
      setTagOptions((current) => mergeTagOptions(current, savedTags))
      setManagedConfig((current) => (current ? { ...current, tags: savedTags } : current))
      setSavedManagedConfig((current) => (current ? { ...current, tags: savedTags } : current))
      managedConfigDirtyRef.current = false
      message.success('区域账号标签已保存')
      await loadAgents({ silent: true })
      return true
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '保存区域标签失败')
      return false
    } finally {
      setTagSaving(false)
    }
  }

  async function createTagOption() {
    const parsed = parseTagInput(newTagName)
    if (!parsed.length) {
      message.warning('请输入标签名称')
      return
    }
    if (!canManageSystem) {
      const nextTags = mergeTagOptions(tagOptions, parsed)
      setTagOptions(nextTags)
      updateManagedConfig((current) => ({ ...current, tags: mergeTagOptions(current.tags || [], parsed) }))
      setNewTagName('')
      message.success('标签已加入当前区域草稿，请保存后生效')
      return
    }
    const nextTags = mergeTagOptions(tagOptions, parsed)
    const ok = await saveTagOptions(nextTags)
    if (ok) {
      setNewTagName('')
      message.success('标签已创建')
    }
  }

  async function saveManagedConfigSection(section: ConfigSectionKey, draftOverride?: ManagedAgentConfig) {
    const draftConfig = draftOverride || managedConfig
    if (!selectedAgentId || !draftConfig) {
      return
    }
    const baseConfig = savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const payload = buildSectionSavePayload(baseConfig, draftConfig, section, selectedAgentId)
    setConfigSavingSection(section)
    setConfigError('')
    try {
      const saved = await fetchJSON<ManagedAgentConfig>(`/api/v1/agents/${selectedAgentId}/config`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const savedMissingCustomerDisplayName = section === 'client' && !hasCustomerDisplayNameField(saved)
      const savedWithSubmittedCustomerName = savedMissingCustomerDisplayName
        ? { ...saved, customer_display_name: payload.customer_display_name || '' }
        : saved
      const normalized = normalizeManagedConfig(
        savedWithSubmittedCustomerName,
        selectedAgentId,
        saved.agent_name || selectedAgent?.agent_name,
      )
      rememberCustomerDisplayName(selectedAgentId, normalized.customer_display_name || '')
      setSavedManagedConfig(normalized)
      const nextDraft = mergeSavedSectionIntoDraft(draftConfig, normalized, section)
      managedConfigDirtyRef.current = configSignature(nextDraft) !== configSignature(normalized)
      setManagedConfig(nextDraft)
      if (section === 'client') {
        setTagOptions((current) => mergeTagOptions(current, normalized.tags || []))
      }
      if (section === 'entry') {
        setEntryAddressInputText(formatAddressInput(normalized.entry?.addresses))
        await loadOverview(selectedAgentId, { silent: true })
      }
      if (savedMissingCustomerDisplayName && payload.customer_display_name) {
        message.warning('用户展示名称已保留在当前页面；用户侧显示需要后端服务更新到新版后生效')
      } else {
        message.success(`${configSectionLabel(section)}已保存，已通过 WS 通知 Client 立即生效；离线时下次轮询生效`)
      }
      await loadAgents()
      await loadConfigAudits(selectedAgentId, { silent: true })
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      const detail = error instanceof Error ? error.message : '保存配置失败'
      setConfigError(detail)
      message.error(detail)
    } finally {
      setConfigSavingSection(null)
    }
  }

  function savePrimaryDomain(value: string) {
    if (!selectedAgentId) {
      return
    }
    const draftConfig =
      managedConfig || savedManagedConfig || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
    const nextDraft = normalizeManagedConfig(
      {
        ...draftConfig,
        entry: {
          ...draftConfig.entry,
          import_domain: value,
        },
      },
      selectedAgentId,
      selectedAgent?.agent_name,
    )
    managedConfigDirtyRef.current = true
    setManagedConfig(nextDraft)
    void saveManagedConfigSection('entry', nextDraft)
  }

  function updateManagedConfig(updater: (current: ManagedAgentConfig) => ManagedAgentConfig) {
    managedConfigDirtyRef.current = true
    setManagedConfig((current) => {
      const base = current || createEmptyManagedConfig(selectedAgentId, selectedAgent?.agent_name)
      return updater(base)
    })
  }

  return {
    overview,
    setOverview,
    overviewLoading,
    setOverviewLoading,
    overviewError,
    setOverviewError,
    managedConfig,
    setManagedConfig,
    savedManagedConfig,
    setSavedManagedConfig,
    managedConfigDirtyRef,
    newTagName,
    setNewTagName,
    tagSaving,
    setTagSaving,
    entryAddressInputText,
    setEntryAddressInputText,
    configLoading,
    setConfigLoading,
    configSavingSection,
    setConfigSavingSection,
    configError,
    setConfigError,
    configAudits,
    setConfigAudits,
    configAuditsLoading,
    setConfigAuditsLoading,
    xuiActions,
    setXUIActions,
    xuiActionsLoading,
    setXUIActionsLoading,
    agentLogs,
    setAgentLogs,
    agentLogsLoading,
    setAgentLogsLoading,
    agentLogsError,
    setAgentLogsError,
    lastDetailAgentIdRef,
    isCurrentAgentRequest,
    loadOverview,
    loadManagedConfig,
    loadXUIActions,
    loadAgentLogs,
    loadConfigAudits,
    loadTagSettings,
    saveTagOptions,
    saveAreaAgentTags,
    createTagOption,
    saveManagedConfigSection,
    savePrimaryDomain,
    updateManagedConfig,
  }
}
