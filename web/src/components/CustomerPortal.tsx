import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Checkbox, Empty, Input, Modal, Pagination, Progress, QRCode, Segmented, Space, Spin, Statistic, Tag, Typography } from 'antd'
import { BarChartOutlined, CheckCircleOutlined, CheckOutlined, CloseCircleOutlined, CloseOutlined, CopyOutlined, DashboardOutlined, DownOutlined, EditOutlined, InfoCircleOutlined, LinkOutlined, LockOutlined, LogoutOutlined, ReloadOutlined, RightOutlined, UnorderedListOutlined, WarningOutlined } from '@ant-design/icons'

import type { CustomerAuthResponse, CustomerDailyLinkUsage, CustomerDailyUsage, CustomerLinkStep, CustomerLinkView, CustomerOverviewResponse, CustomerTrafficRecord, CustomerUsageResponse, CustomerUser } from '../types'
import { countryFlag, fetchJSON, formatDateTime } from '../lib/appHelpers'
import { formatBytes } from '../lib/traffic'
import { LoginScreen } from './LoginScreen'
import { CustomerSupportWidget } from './CustomerSupportWidget'
import { clearCustomFrontendCode } from './VisualEffects'
import { CustomerThemeButton } from './CustomerThemeButton'

const { Paragraph, Text, Title } = Typography

export function CustomerPortal() {
  const { message } = AntdApp.useApp()
  const [sessionLoading, setSessionLoading] = useState(true)
  const [loginLoading, setLoginLoading] = useState(false)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [usageLoading, setUsageLoading] = useState(false)
  const [usageRange, setUsageRange] = useState<'7d' | '30d'>('30d')
  const [usageFrom, setUsageFrom] = useState('')
  const [usageTo, setUsageTo] = useState('')
  const [usage, setUsage] = useState<CustomerUsageResponse | null>(null)
  const [activeMenu, setActiveMenu] = useState<'overview' | 'usage' | 'links'>('overview')
  const [savingRemarkID, setSavingRemarkID] = useState<number | null>(null)
  const [passwordModalOpen, setPasswordModalOpen] = useState(false)
  const [passwordSaving, setPasswordSaving] = useState(false)
  const [subscriptionModalOpen, setSubscriptionModalOpen] = useState(false)
  const [selectedSubscriptionAssignmentIDs, setSelectedSubscriptionAssignmentIDs] = useState<number[]>([])
  const [passwordForm, setPasswordForm] = useState({
    current_password: '',
    new_password: '',
    confirm_password: '',
  })
  const [qrLink, setQrLink] = useState<CustomerLinkView | null>(null)
  const [announcementModalOpen, setAnnouncementModalOpen] = useState(false)
  const [announcementIndex, setAnnouncementIndex] = useState(0)
  const [readAnnouncementIDs, setReadAnnouncementIDs] = useState<string[]>([])
  const [editingRemarkID, setEditingRemarkID] = useState<number | null>(null)
  const [user, setUser] = useState<CustomerUser | null>(null)
  const [overview, setOverview] = useState<CustomerOverviewResponse | null>(null)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [remarkDrafts, setRemarkDrafts] = useState<Record<number, string>>({})
  const lastAnnouncementSetRef = useRef('')

  useEffect(() => {
    const username = user?.username || ''
    if (!username) {
      setReadAnnouncementIDs([])
      return
    }
    try {
      const stored = window.localStorage.getItem(customerAnnouncementReadKey(username))
      const parsed = stored ? JSON.parse(stored) : []
      setReadAnnouncementIDs(Array.isArray(parsed) ? parsed.filter((value): value is string => typeof value === 'string') : [])
    } catch {
      setReadAnnouncementIDs([])
    }
  }, [user?.username])

  useEffect(() => {
    const serverReadIDs = overview?.read_announcement_ids || []
    if (serverReadIDs.length) {
      setReadAnnouncementIDs((current) => Array.from(new Set([...current, ...serverReadIDs])))
    }
  }, [overview?.read_announcement_ids])

  useEffect(() => {
    document.title = 'ZaneLin Customer'
    clearCustomFrontendCode()
    return () => {
      document.title = 'ZaneLin'
      clearCustomFrontendCode()
    }
  }, [])

  useEffect(() => {
    void loadSession()
  }, [])

  useEffect(() => {
    const announcements = overview?.announcements || []
    const setKey = JSON.stringify(announcements.map((item) => [item.id, item.level, item.title, item.content, item.link_label, item.link_url]))
    if (announcements.length === 0) {
      setAnnouncementModalOpen(false)
      lastAnnouncementSetRef.current = ''
      return
    }
    // The server response is already available in this render; the effect
    // that copies its read IDs into local state runs after this render.
    const readIDs = new Set([...readAnnouncementIDs, ...(overview?.read_announcement_ids || [])])
    const firstUnreadIndex = announcements.findIndex((item) => !readIDs.has(item.id))
    if (firstUnreadIndex >= 0 && setKey !== lastAnnouncementSetRef.current) {
      lastAnnouncementSetRef.current = setKey
      setAnnouncementIndex(firstUnreadIndex)
      setAnnouncementModalOpen(true)
    }
  }, [overview?.announcements, overview?.read_announcement_ids, readAnnouncementIDs])

  const exitCountryCount = useMemo(() => {
    const values = new Set<string>()
    for (const link of overview?.links || []) {
      const country = link.exit_country_code || link.exit_country_name
      if (country) {
        values.add(country)
      }
    }
    return values.size
  }, [overview?.links])

  async function loadSession() {
    setSessionLoading(true)
    try {
      const data = await fetchJSON<CustomerAuthResponse>('/api/v1/customer/session')
      setUser(data.user)
      await loadOverview()
      await loadUsage('30d')
    } catch {
      setUser(null)
      setOverview(null)
    } finally {
      setSessionLoading(false)
    }
  }

  async function login() {
    setLoginLoading(true)
    try {
      const data = await fetchJSON<CustomerAuthResponse>('/api/v1/customer/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(loginForm),
      })
      setUser(data.user)
      setLoginForm({ username: data.user.username, password: '' })
      await loadOverview()
      await loadUsage('30d')
      message.success('登录成功')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setLoginLoading(false)
    }
  }

  async function logout() {
    try {
      await fetchJSON<{ status: string }>('/api/v1/customer/logout', { method: 'POST' })
    } catch {
      // Local state is cleared even if the cookie cleanup request fails.
    }
    setUser(null)
    setOverview(null)
    setUsage(null)
    setActiveMenu('overview')
    setRemarkDrafts({})
    setAnnouncementModalOpen(false)
    setAnnouncementIndex(0)
    lastAnnouncementSetRef.current = ''
  }

  function confirmAnnouncementsRead() {
    const announcements = overview?.announcements || []
    const username = user?.username || ''
    const ids = announcements.map((item) => item.id).filter(Boolean)
    if (username && ids.length) {
      const next = Array.from(new Set([...readAnnouncementIDs, ...ids]))
      setReadAnnouncementIDs(next)
      try {
        window.localStorage.setItem(customerAnnouncementReadKey(username), JSON.stringify(next))
      } catch {
        // Remembering read state is optional; the current session still closes.
      }
      void fetchJSON('/api/v1/customer/announcements/read', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ announcement_ids: ids }),
      }).catch(() => {
        // Keep the local state when the server acknowledgement is temporarily unavailable.
      })
    }
    setAnnouncementModalOpen(false)
  }

  async function loadOverview() {
    setOverviewLoading(true)
    try {
      const data = await fetchJSON<CustomerOverviewResponse>('/api/v1/customer/overview')
      setOverview(data)
      setRemarkDrafts(Object.fromEntries(data.links.map((link) => [link.assignment_id, link.remark || ''])))
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message)
      }
    } finally {
      setOverviewLoading(false)
    }
  }

  async function loadUsage(range: '7d' | '30d' = usageRange, from = usageFrom, to = usageTo) {
    setUsageLoading(true)
    try {
      const query = from && to ? `from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : `range=${range}`
      const data = await fetchJSON<CustomerUsageResponse>(`/api/v1/customer/usage?${query}`)
      setUsage(data)
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message)
      }
    } finally {
      setUsageLoading(false)
    }
  }

  async function refreshCustomerData() {
    await Promise.all([loadOverview(), loadUsage(usageRange)])
  }

  async function saveRemark(link: CustomerLinkView) {
    setSavingRemarkID(link.assignment_id)
    try {
      const remark = remarkDrafts[link.assignment_id] || ''
      await fetchJSON(`/api/v1/customer/assignments/${link.assignment_id}/remark`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ remark }),
      })
      setOverview((current) => current ? {
        ...current,
        links: current.links.map((item) => item.assignment_id === link.assignment_id ? { ...item, remark } : item),
      } : current)
      setEditingRemarkID(null)
      message.success('备注已保存')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存备注失败')
    } finally {
      setSavingRemarkID(null)
    }
  }

  function openPasswordModal() {
    setPasswordForm({
      current_password: '',
      new_password: '',
      confirm_password: '',
    })
    setPasswordModalOpen(true)
  }

  async function saveCustomerPassword() {
    if (!passwordForm.current_password || !passwordForm.new_password) {
      message.warning('请填写当前密码和新密码')
      return
    }
    if (passwordForm.new_password !== passwordForm.confirm_password) {
      message.error('两次输入的新密码不一致')
      return
    }
    setPasswordSaving(true)
    try {
      const data = await fetchJSON<CustomerAuthResponse>('/api/v1/customer/account', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          current_password: passwordForm.current_password,
          new_password: passwordForm.new_password,
        }),
      })
      setUser(data.user)
      setPasswordModalOpen(false)
      setPasswordForm({
        current_password: '',
        new_password: '',
        confirm_password: '',
      })
      message.success('密码已修改')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '修改密码失败')
    } finally {
      setPasswordSaving(false)
    }
  }

  async function copyImportURL(link: CustomerLinkView) {
    if (!link.import_url) {
      message.warning('当前链路没有可复制的客户端链接')
      return
    }
    try {
      await navigator.clipboard.writeText(customerLinkImportURL(link, remarkDrafts[link.assignment_id]))
      message.success('客户端链接已复制')
    } catch {
      message.error('复制失败，请手动复制')
    }
  }

  function openClashSubscriptionModal() {
    const subscriptionURL = overview?.clash_subscription_url || overview?.mihomo_subscription_url || ''
    if (!subscriptionURL) {
      message.warning('当前服务端暂未返回订阅地址，请先发布新版 Server')
      return
    }
    setSelectedSubscriptionAssignmentIDs([])
    setSubscriptionModalOpen(true)
  }

  function closeSubscriptionModal() {
    setSubscriptionModalOpen(false)
    setSelectedSubscriptionAssignmentIDs([])
  }

  async function copyClashSubscriptionURL(assignmentIDs: number[]) {
    if (assignmentIDs.length === 0) {
      message.warning('请至少选择一个导出节点')
      return
    }
    const baseSubscriptionURL = overview?.clash_subscription_url || overview?.mihomo_subscription_url || ''
    if (!baseSubscriptionURL) {
      message.warning('当前服务端暂未返回订阅地址，请先发布新版 Server')
      return
    }
    try {
      const subscriptionURL = new URL(baseSubscriptionURL, window.location.origin)
      subscriptionURL.searchParams.set('assignments', assignmentIDs.join(','))
      await navigator.clipboard.writeText(subscriptionURL.toString())
      message.success('Clash/Mihomo 订阅已复制')
      closeSubscriptionModal()
    } catch {
      message.error('复制失败，请手动复制')
    }
  }

  if (sessionLoading) {
    return (
      <div className="login-shell">
        <Spin size="large" />
      </div>
    )
  }

  if (!user) {
    return (
      <LoginScreen
        consoleAppearance
        audience="customer"
        title="授权链路面板"
        subtitle="使用授权账号登录，查看链路与订阅"
        loginForm={loginForm}
        loginLoading={loginLoading}
        onChange={setLoginForm}
        onLogin={login}
      />
    )
  }

  return (
    <div className="page-shell customer-page-shell">
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className="customer-shell customer-dashboard-shell">
        <header className="customer-mobile-header">
          <div>
            <div className="eyebrow">授权链路</div>
            <Title level={2}>{user.display_name || user.username}</Title>
            <Text type="secondary">{overview?.generated_at ? formatDateTime(overview.generated_at) : '等待数据同步'}</Text>
          </div>
          <div className="customer-mobile-actions">
            <CustomerThemeButton />
            <Button shape="circle" aria-label="复制 Clash/Mihomo 订阅" icon={<CopyOutlined />} title="复制 Clash/Mihomo 订阅" onClick={openClashSubscriptionModal} />
            <Button shape="circle" aria-label="修改密码" title="修改密码" icon={<LockOutlined />} onClick={openPasswordModal} />
            <Button shape="circle" aria-label="刷新" title="刷新" icon={<ReloadOutlined />} loading={overviewLoading || usageLoading} onClick={() => void refreshCustomerData()} />
            <Button shape="circle" aria-label="退出" title="退出" icon={<LogoutOutlined />} onClick={() => void logout()} />
          </div>
        </header>
        <section className="customer-mobile-summary">
          <div>
            <span>链路</span>
            <strong>{overview?.links.length || 0}</strong>
          </div>
          <div>
            <span>已解析</span>
            <strong>{(overview?.links || []).filter((link) => link.resolved).length}</strong>
          </div>
          <div>
            <span>出口地区</span>
            <strong>{exitCountryCount}</strong>
          </div>
        </section>
        <header className="customer-hero customer-dashboard-header">
          <div>
            <div className="eyebrow">授权访问 / 我的链路</div>
            <Title level={1}>{user.display_name || user.username}</Title>
          </div>
          <Space wrap className="customer-header-actions">
            <CustomerThemeButton />
            <Button icon={<CopyOutlined />} onClick={openClashSubscriptionModal}>复制 Clash/Mihomo 订阅</Button>
            <Button icon={<LockOutlined />} onClick={openPasswordModal}>修改密码</Button>
            <Button icon={<ReloadOutlined />} loading={overviewLoading || usageLoading} onClick={() => void refreshCustomerData()}>刷新</Button>
            <Button icon={<LogoutOutlined />} onClick={() => void logout()}>退出</Button>
          </Space>
        </header>
        <div className="customer-board-layout">
          <aside className="customer-board-side">
            <nav className="customer-section-nav" aria-label="客户面板菜单">
              {([
                { key: 'overview', label: '概览', icon: <DashboardOutlined /> },
                { key: 'usage', label: '流量用量', icon: <BarChartOutlined /> },
                { key: 'links', label: '授权链路', icon: <LinkOutlined /> },
              ] as const).map((item) => (
                <button
                  key={item.key}
                  type="button"
                  className={`customer-section-nav-item${activeMenu === item.key ? ' active' : ''}`}
                  aria-current={activeMenu === item.key ? 'page' : undefined}
                  onClick={() => setActiveMenu(item.key)}
                >
                  <span className="customer-section-nav-icon" aria-hidden="true">{item.icon}</span>
                  <span>{item.label}</span>
                </button>
              ))}
            </nav>
            <Card bordered={false} className="surface-card customer-board-profile">
              <Text type="secondary">看板摘要</Text>
              <div className="customer-board-score">
                <strong>{overview?.links.length || 0}</strong>
                <span>条授权链路</span>
              </div>
              <div className="customer-board-score">
                <strong>{exitCountryCount}</strong>
                <span>个出口地区</span>
              </div>
            </Card>
          </aside>

          <section className="customer-board-main">
            {activeMenu === 'overview' ? (
              <div className="customer-section-view customer-overview-view">
                <div className="customer-section-heading">
                  <div>
                    <Title level={2}>概览</Title>
                    <Text type="secondary">快速查看账号状态、授权规模和最近链路</Text>
                  </div>
                  <Button type="link" onClick={() => setActiveMenu('links')}>查看全部链路</Button>
                </div>
                <Alert
                  className="customer-policy-alert"
                  type="info"
                  showIcon
                  message="授权使用规则"
                  description="仅限已授权用户使用，可按独享或共享方式开放；禁止滥发、攻击、诈骗、爬虫滥用、扫描爆破等高风险行为。发现异常时，管理员可随时停用账号或授权链路，并停用对应 x-ui client。"
                />
                <CustomerAnnouncementBar announcements={overview?.announcements || []} onOpen={(index) => {
                  setAnnouncementIndex(index)
                  setAnnouncementModalOpen(true)
                }} />
                <div className="customer-stat-grid">
                  <Card bordered={false} className="surface-card customer-stat-card">
                    <Statistic title="已授权链路" value={overview?.links.length || 0} suffix="条" />
                  </Card>
                  <Card bordered={false} className="surface-card customer-stat-card">
                    <Statistic title="已解析链路" value={(overview?.links || []).filter((link) => link.resolved).length} suffix="条" />
                  </Card>
                  <Card bordered={false} className="surface-card customer-stat-card">
                    <Statistic title="出口地区" value={exitCountryCount} suffix="个" />
                  </Card>
                </div>
                <Card bordered={false} className="surface-card customer-recent-links-card" title="最近授权链路">
                  <Space direction="vertical" size={10} style={{ width: '100%' }}>
                    {(overview?.links || []).slice(0, 3).map((link) => (
                      <button key={link.assignment_id} type="button" className="customer-recent-link-row" onClick={() => setActiveMenu('links')}>
                        <span>
                          <strong>{customerLinkDisplayName(link, remarkDrafts[link.assignment_id])}</strong>
                          <small>{customerLinkSummaryText(link)}</small>
                        </span>
                        <Tag color={link.resolved ? 'blue' : 'orange'}>{link.resolved ? '已解析' : '待解析'}</Tag>
                      </button>
                    ))}
                    {!(overview?.links || []).length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无授权链路，请联系管理员开通" /> : null}
                  </Space>
                </Card>
              </div>
            ) : null}

            {activeMenu === 'usage' ? (
              <div className="customer-section-view">
                <div className="customer-section-heading">
                  <div>
                    <Title level={2}>流量用量</Title>
                    <Text type="secondary">按当前授权链路汇总上传、下载和每日消耗</Text>
                  </div>
                </div>
                <CustomerUsagePanel
                  overview={overview}
                  usage={usage}
                  range={usageRange}
                  dateFrom={usageFrom}
                  dateTo={usageTo}
                  loading={usageLoading}
                  onRangeChange={(nextRange) => {
                    setUsageRange(nextRange)
                    setUsageFrom('')
                    setUsageTo('')
                    void loadUsage(nextRange, '', '')
                  }}
                  onDateRangeChange={(from, to) => {
                    setUsageFrom(from)
                    setUsageTo(to)
                    void loadUsage(usageRange, from, to)
                  }}
                />
              </div>
            ) : null}

            {activeMenu === 'links' ? (
              <div className="customer-section-view customer-links-view">
                <div className="customer-section-heading">
                  <div>
                    <Title level={2}>授权链路</Title>
                    <Text type="secondary">查看链路拓扑、复制订阅或点击二维码查看大图</Text>
                  </div>
                  <Button icon={<CopyOutlined />} onClick={openClashSubscriptionModal}>复制订阅</Button>
                </div>
                <Spin spinning={overviewLoading}>
                  <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                    {overview?.generated_at ? <Text type="secondary">数据更新时间：{formatDateTime(overview.generated_at)}</Text> : null}
                    {!overview?.links.length ? (
                      <Card bordered={false} className="surface-card customer-link-card">
                        <Empty description="暂无授权链路，请联系管理员开通" />
                      </Card>
                    ) : null}
                    {(overview?.links || []).map((link) => {
                      const effectiveName = customerLinkDisplayName(link, remarkDrafts[link.assignment_id])
                      const effectiveImportURL = customerLinkImportURL(link, remarkDrafts[link.assignment_id])
                      const billingItems = customerLinkMetaItems(link)
                      return (
                      <Card key={link.assignment_id} bordered={false} className="surface-card customer-link-card">
                        <div className="customer-link-head">
                          <div className="customer-link-main">
                            <div className="customer-link-title-row">
                              {editingRemarkID === link.assignment_id ? (
                                <Input
                                  className="customer-link-remark-input"
                                  value={remarkDrafts[link.assignment_id] || ''}
                                  placeholder={link.entry_client_name}
                                  maxLength={120}
                                  autoFocus
                                  onChange={(event) => setRemarkDrafts((current) => ({ ...current, [link.assignment_id]: event.target.value }))}
                                  onPressEnter={() => void saveRemark(link)}
                                />
                              ) : (
                                <Title level={3}>{effectiveName}</Title>
                              )}
                              {editingRemarkID === link.assignment_id ? (
                                <Space size={4}>
                                  <Button size="small" type="primary" icon={<CheckOutlined />} loading={savingRemarkID === link.assignment_id} onClick={() => void saveRemark(link)} />
                                  <Button size="small" icon={<CloseOutlined />} onClick={() => {
                                    setRemarkDrafts((current) => ({ ...current, [link.assignment_id]: link.remark || '' }))
                                    setEditingRemarkID(null)
                                  }} />
                                </Space>
                              ) : (
                                <Button
                                  size="small"
                                  type="text"
                                  className="customer-remark-edit-button"
                                  icon={<EditOutlined />}
                                  title="修改备注，导入名称会同步更新"
                                  onClick={() => {
                                    setRemarkDrafts((current) => ({ ...current, [link.assignment_id]: link.remark || '' }))
                                    setEditingRemarkID(link.assignment_id)
                                  }}
                                />
                              )}
                            </div>
                            <Space wrap className="customer-link-exit-tags">
                              {link.exit_country_code || link.exit_country_name ? <Tag color="blue">出口 {countryFlag(link.exit_country_code)} {link.exit_country_code || link.exit_country_name}</Tag> : null}
                              {link.exit_ip ? <Tag color="geekblue">{link.exit_ip}</Tag> : null}
                              {!link.resolved ? <Tag color="orange">待解析</Tag> : null}
                              <Button
                                type="primary"
                                size="small"
                                className="customer-link-copy-button"
                                icon={<CopyOutlined />}
                                disabled={!effectiveImportURL}
                                onClick={() => void copyImportURL(link)}
                              >
                                复制链接
                              </Button>
                            </Space>
                            {billingItems.length ? (
                              <div className="customer-link-meta-row" aria-label="授权链路费用和过期时间">
                                {billingItems.map((item) => (
                                  item.expiry ? (
                                    <div key={item.key} className={`customer-link-meta-item customer-link-meta-${item.key} customer-link-meta-expiry-detail`}>
                                      <div className="customer-link-meta-expiry-head">
                                        <span>{item.text}</span>
                                        <strong>{item.expiry.label}</strong>
                                      </div>
                                      {item.expiry.percent !== undefined ? (
                                        <Progress
                                          percent={item.expiry.percent}
                                          showInfo={false}
                                          size="small"
                                          strokeColor={item.expiry.strokeColor}
                                        />
                                      ) : null}
                                    </div>
                                  ) : (
                                    <span key={item.key} className={`customer-link-meta-item customer-link-meta-${item.key}`}>{item.text}</span>
                                  )
                                ))}
                              </div>
                            ) : null}
                          </div>
                          <CustomerLinkTraffic link={link} />
                        </div>

                        <div className="customer-link-visual-row">
                          <div className="customer-topology-scroll">
                            <CustomerTopologyMap steps={link.steps} />
                          </div>
                          <aside className="customer-qr-inline">
                            {effectiveImportURL ? (
                              <button type="button" className="customer-qr-button" onClick={() => setQrLink(link)}>
                                <QRCode value={effectiveImportURL} color="#171717" bgColor="#ffffff" bordered={false} size={132} />
                                <span>点击放大</span>
                              </button>
                            ) : (
                              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无二维码" />
                            )}
                            <Text type="secondary" className="customer-link-import-hint">导入名称：{effectiveName}</Text>
                          </aside>
                        </div>
                      </Card>
                      )
                    })}
                  </Space>
                </Spin>
              </div>
            ) : null}
          </section>
        </div>
      </div>

      <CustomerAnnouncementModal
        announcements={overview?.announcements || []}
        index={announcementIndex}
        open={announcementModalOpen}
        onIndexChange={setAnnouncementIndex}
        onClose={() => setAnnouncementModalOpen(false)}
        onConfirmRead={confirmAnnouncementsRead}
      />

      <CustomerSupportWidget />

      <Modal
        className="customer-subscription-modal"
        title="选择 Clash/Mihomo 订阅节点"
        open={subscriptionModalOpen}
        onCancel={closeSubscriptionModal}
        footer={(
          <Space>
            <Button onClick={closeSubscriptionModal}>取消</Button>
            <Button
              type="primary"
              disabled={selectedSubscriptionAssignmentIDs.length === 0}
              onClick={() => void copyClashSubscriptionURL(selectedSubscriptionAssignmentIDs)}
            >
              复制订阅地址
            </Button>
          </Space>
        )}
        width={680}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text strong>选择导出的节点</Text>
            <div className="muted-line">默认全部不选；只会导出下方勾选且已解析的授权链路。</div>
          </div>
          <Space wrap>
            <Button
              size="small"
              disabled={!(overview?.links || []).some((link) => link.resolved && link.import_url)}
              onClick={() => setSelectedSubscriptionAssignmentIDs((overview?.links || []).filter((link) => link.resolved && link.import_url).map((link) => link.assignment_id))}
            >
              全选
            </Button>
            <Button size="small" onClick={() => setSelectedSubscriptionAssignmentIDs([])}>清空</Button>
            <Text type="secondary">已选择 {selectedSubscriptionAssignmentIDs.length} / {(overview?.links || []).filter((link) => link.resolved && link.import_url).length} 条</Text>
          </Space>
          <Checkbox.Group
            style={{ width: '100%' }}
            value={selectedSubscriptionAssignmentIDs}
            onChange={(values) => setSelectedSubscriptionAssignmentIDs(values.map((value) => Number(value)))}
          >
            <Space direction="vertical" size={10} style={{ width: '100%' }}>
              {(overview?.links || []).map((link) => (
                <Checkbox key={link.assignment_id} value={link.assignment_id} disabled={!link.resolved || !link.import_url}>
                  <div>
                    <Text strong>{customerLinkDisplayName(link, remarkDrafts[link.assignment_id])}</Text>
                    <div className="muted-line">{customerLinkSummaryText(link)}{link.resolved && link.import_url ? '' : '（暂不可导出）'}</div>
                  </div>
                </Checkbox>
              ))}
            </Space>
          </Checkbox.Group>
          {!(overview?.links || []).length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无授权链路" /> : null}
        </Space>
      </Modal>

      <Modal
        title={qrLink ? `${customerLinkDisplayName(qrLink, remarkDrafts[qrLink.assignment_id])} 二维码` : '客户端二维码'}
        className="customer-qr-modal"
        open={Boolean(qrLink)}
        onCancel={() => setQrLink(null)}
        footer={[
          <Button key="close" onClick={() => setQrLink(null)}>关闭</Button>,
          <Button key="copy" type="primary" disabled={!qrLink?.import_url} onClick={() => qrLink && void copyImportURL(qrLink)}>复制链接</Button>,
        ]}
      >
        {qrLink?.import_url ? (
          <Space direction="vertical" size="middle" style={{ width: '100%', alignItems: 'center' }}>
            <div className="customer-qr-frame">
              <QRCode value={customerLinkImportURL(qrLink, remarkDrafts[qrLink.assignment_id])} color="#171717" bgColor="#ffffff" size={260} bordered={false} />
            </div>
            <Text type="secondary">导入名称：{customerLinkDisplayName(qrLink, remarkDrafts[qrLink.assignment_id])}</Text>
          </Space>
        ) : <Empty description="暂无二维码" />}
      </Modal>

      <Modal
        title="修改密码"
        open={passwordModalOpen}
        onCancel={() => setPasswordModalOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setPasswordModalOpen(false)}>取消</Button>,
          <Button key="save" type="primary" loading={passwordSaving} onClick={() => void saveCustomerPassword()}>保存新密码</Button>,
        ]}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="修改后会保留当前登录状态"
            description="为了账号安全，其他浏览器或设备上的登录会失效，需要使用新密码重新登录。"
          />
          <Input.Password
            value={passwordForm.current_password}
            placeholder="当前密码"
            autoComplete="current-password"
            onChange={(event) => setPasswordForm((current) => ({ ...current, current_password: event.target.value }))}
          />
          <Input.Password
            value={passwordForm.new_password}
            placeholder="新密码，至少 8 位"
            autoComplete="new-password"
            onChange={(event) => setPasswordForm((current) => ({ ...current, new_password: event.target.value }))}
          />
          <Input.Password
            value={passwordForm.confirm_password}
            placeholder="再次输入新密码"
            autoComplete="new-password"
            onChange={(event) => setPasswordForm((current) => ({ ...current, confirm_password: event.target.value }))}
            onPressEnter={() => void saveCustomerPassword()}
          />
        </Space>
      </Modal>
    </div>
  )
}

function CustomerAnnouncementModal(props: {
  announcements: NonNullable<CustomerOverviewResponse['announcements']>
  index: number
  open: boolean
  onIndexChange: (index: number) => void
  onClose: () => void
  onConfirmRead: () => void
}) {
  const { announcements, index, open, onIndexChange, onClose, onConfirmRead } = props
  const announcement = announcements[index]
  if (!announcement) return null
  const level = announcement.level || 'info'
  const linkURL = safeCustomerAnnouncementURL(announcement.link_url)
  const isLast = index >= announcements.length - 1

  return (
    <Modal
      className={`customer-announcement-modal customer-announcement-modal-${level}`}
      title={(
        <span className={`customer-announcement-title customer-announcement-title-${level}`}>
          {customerAnnouncementIcon(level)}
          <span>{announcement.title}</span>
        </span>
      )}
      open={open}
      centered
      width={560}
      onCancel={onClose}
      footer={[
        announcements.length > 1 ? <Text key="count" type="secondary" className="customer-announcement-count">{index + 1} / {announcements.length}</Text> : null,
        index > 0 ? <Button key="previous" onClick={() => onIndexChange(index - 1)}>上一条</Button> : null,
        !isLast ? <Button key="next" type="primary" onClick={() => onIndexChange(index + 1)}>下一条</Button> : null,
        isLast ? <Button key="close" type="primary" onClick={onConfirmRead}>我知道了</Button> : null,
      ]}
    >
      {announcement.content ? <Paragraph className="customer-announcement-content">{announcement.content}</Paragraph> : null}
      {linkURL ? (
        <Button type="primary" href={linkURL} target="_blank" rel="noreferrer">
          {announcement.link_label || '查看新联系方式'}
        </Button>
      ) : null}
    </Modal>
  )
}

function CustomerUsagePanel(props: {
  overview: CustomerOverviewResponse | null
  usage: CustomerUsageResponse | null
  range: '7d' | '30d'
  dateFrom: string
  dateTo: string
  loading: boolean
  onRangeChange: (range: '7d' | '30d') => void
  onDateRangeChange: (from: string, to: string) => void
}) {
  const [historyView, setHistoryView] = useState<'chart' | 'list'>('chart')
  const [expandedDailyDates, setExpandedDailyDates] = useState<string[]>([])
  const [customDateFrom, setCustomDateFrom] = useState(props.dateFrom)
  const [customDateTo, setCustomDateTo] = useState(props.dateTo)
  const [dailyPage, setDailyPage] = useState(1)
  const [recordPage, setRecordPage] = useState(1)
  const fallback = customerUsageFallback(props.overview)
  const summary = props.usage || fallback
  const daily = props.usage?.daily || []
  const records = props.usage?.records || []
  const linksByAssignmentID = useMemo(() => new Map((props.overview?.links || []).map((link) => [link.assignment_id, link])), [props.overview?.links])
  const dailyPageSize = 7
  const recordPageSize = 20
  const visibleDaily = daily.slice((dailyPage - 1) * dailyPageSize, dailyPage * dailyPageSize)
  const visibleRecords = records.slice((recordPage - 1) * recordPageSize, recordPage * recordPageSize)

  useEffect(() => {
    setCustomDateFrom(props.dateFrom)
    setCustomDateTo(props.dateTo)
  }, [props.dateFrom, props.dateTo])

  useEffect(() => {
    setDailyPage(1)
    setRecordPage(1)
    setExpandedDailyDates([])
  }, [props.usage])

  const toggleDailyDate = (date: string) => {
    setExpandedDailyDates((current) => current.includes(date) ? current.filter((value) => value !== date) : [...current, date])
  }

  return (
    <Card bordered={false} className="surface-card customer-usage-panel">
      <div className="customer-usage-header">
        <div>
          <Title level={3}>流量用量</Title>
          <Text type="secondary">上传、下载和每日消耗，按当前授权链路汇总</Text>
        </div>
        <Space wrap>
          <Segmented
            value={props.range}
            options={[{ label: '近 7 天', value: '7d' }, { label: '近 30 天', value: '30d' }]}
            onChange={(value) => props.onRangeChange(value as '7d' | '30d')}
          />
          <Segmented
            value={historyView}
            aria-label="历史记录查看方式"
            options={[
              { label: <span><BarChartOutlined /> 每日用量</span>, value: 'chart' },
              { label: <span><UnorderedListOutlined /> 最近记录</span>, value: 'list' },
            ]}
            onChange={(value) => setHistoryView(value as 'chart' | 'list')}
          />
          <Space size={6} className="customer-usage-date-filter">
            <input aria-label="开始日期" type="date" value={customDateFrom} onChange={(event) => setCustomDateFrom(event.target.value)} />
            <span aria-hidden="true">至</span>
            <input aria-label="结束日期" type="date" value={customDateTo} onChange={(event) => setCustomDateTo(event.target.value)} />
            <Button
              size="small"
              type="primary"
              disabled={!customDateFrom || !customDateTo || customDateFrom > customDateTo}
              onClick={() => props.onDateRangeChange(customDateFrom, customDateTo)}
            >
              查询
            </Button>
          </Space>
        </Space>
      </div>

      <Spin spinning={props.loading}>
        <div className="customer-usage-summary-grid">
          <div className="customer-usage-summary-item customer-usage-summary-primary">
            <Text type="secondary">本周期已用</Text>
            <strong>{formatBytes(summary.counted_total_bytes)}</strong>
          </div>
          <div className="customer-usage-summary-item">
            <Text type="secondary">上传</Text>
            <strong>{formatBytes(summary.upload_bytes)}</strong>
          </div>
          <div className="customer-usage-summary-item">
            <Text type="secondary">下载</Text>
            <strong>{formatBytes(summary.download_bytes)}</strong>
          </div>
          <div className="customer-usage-summary-item">
            <Text type="secondary">可用总流量</Text>
            <strong>{summary.unlimited ? '无限量' : formatBytes(summary.quota_bytes)}</strong>
          </div>
        </div>

        {historyView === 'chart' ? (
          <>
            {daily.length ? (
              <div className="customer-usage-table-wrap">
                <table className="customer-usage-table">
                  <thead><tr><th>每日用量</th><th>上传</th><th>下载</th><th>合计</th><th>计费用量</th></tr></thead>
                  <tbody>{visibleDaily.map((item: CustomerDailyUsage) => {
                    const linkUsage = item.links || []
                    const expanded = expandedDailyDates.includes(item.date)
                    return (
                      <Fragment key={`daily-${item.date}`}>
                        <tr className={linkUsage.length ? 'customer-usage-daily-row customer-usage-daily-row-expandable' : 'customer-usage-daily-row'}>
                          <td>
                            {linkUsage.length ? (
                              <button
                                type="button"
                                className="customer-usage-expand-button"
                                aria-label={`${expanded ? '收起' : '展开'} ${item.date} 的链路用量`}
                                aria-expanded={expanded}
                                onClick={() => toggleDailyDate(item.date)}
                              >
                                {expanded ? <DownOutlined /> : <RightOutlined />}
                                <span>{item.date}</span>
                              </button>
                            ) : item.date}
                          </td>
                          <td>{formatBytes(item.upload_bytes)}</td>
                          <td>{formatBytes(item.download_bytes)}</td>
                          <td>{formatBytes(item.total_bytes)}</td>
                          <td>{formatBytes(item.counted_bytes)}</td>
                        </tr>
                        {expanded ? (
                          <tr key={`detail-${item.date}-links`} className="customer-usage-daily-details-row">
                            <td colSpan={5}>
                              <div className="customer-usage-daily-details" aria-label={`${item.date} 链路用量明细`}>
                                <div className="customer-usage-daily-details-title">链路用量明细</div>
                                <div className="customer-usage-daily-details-head">
                                  <span>链路</span><span>上传</span><span>下载</span><span>合计</span><span>计费用量</span>
                                </div>
                                {linkUsage.map((linkItem: CustomerDailyLinkUsage) => {
                                  const link = linksByAssignmentID.get(linkItem.assignment_id)
                                  return (
                                    <div key={`${item.date}-${linkItem.assignment_id}`} className="customer-usage-daily-details-item">
                                      <span className="customer-usage-daily-details-name">{link ? customerLinkDisplayName(link) : `链路 #${linkItem.assignment_id}`}</span>
                                      <span>{formatBytes(linkItem.upload_bytes)}</span>
                                      <span>{formatBytes(linkItem.download_bytes)}</span>
                                      <span>{formatBytes(linkItem.total_bytes)}</span>
                                      <span>{formatBytes(linkItem.counted_bytes)}</span>
                                    </div>
                                  )
                                })}
                              </div>
                            </td>
                          </tr>
                        ) : null}
                      </Fragment>
                    )
                  })}</tbody>
                </table>
                {daily.length > dailyPageSize ? (
                  <Pagination
                    className="customer-usage-pagination"
                    current={dailyPage}
                    pageSize={dailyPageSize}
                    total={daily.length}
                    showSizeChanger={false}
                    size="small"
                    onChange={(page) => setDailyPage(page)}
                  />
                ) : null}
              </div>
            ) : null}
          </>
        ) : (
          <div className="customer-usage-history-list" aria-label="最近流量记录">
            <div className="customer-usage-chart-head">
              <Text strong>最近流量记录</Text>
              <Text type="secondary">仅保留最近 3 天 · 上传 / 下载</Text>
            </div>
            {records.length ? visibleRecords.map((item: CustomerTrafficRecord) => (
              <div key={`history-${item.recorded_at}`} className="customer-usage-history-row">
                <Text className="customer-usage-history-date">{formatCustomerTrafficRecordTime(item.recorded_at)}</Text>
                <div className="customer-usage-history-values">
                  <span className="customer-usage-history-upload">↑ {formatBytes(item.upload_bytes)}</span>
                  <span className="customer-usage-history-download">↓ {formatBytes(item.download_bytes)}</span>
                </div>
              </div>
            )) : daily.length ? [...daily].reverse().map((item) => (
              <div key={`history-${item.date}`} className="customer-usage-history-row">
                <Text className="customer-usage-history-date">{item.date}</Text>
                <div className="customer-usage-history-values">
                  <span className="customer-usage-history-upload">↑ {formatBytes(item.upload_bytes)}</span>
                  <span className="customer-usage-history-download">↓ {formatBytes(item.download_bytes)}</span>
                </div>
              </div>
            )) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无流量历史记录，等待下一次采集" />
            )}
            {records.length > recordPageSize ? (
              <Pagination
                className="customer-usage-pagination"
                current={recordPage}
                pageSize={recordPageSize}
                total={records.length}
                showSizeChanger={false}
                size="small"
                onChange={(page) => setRecordPage(page)}
              />
            ) : null}
          </div>
        )}
      </Spin>
    </Card>
  )
}

function customerUsageFallback(overview: CustomerOverviewResponse | null): CustomerUsageResponse {
  let upload = 0
  let download = 0
  let counted = 0
  let quota = 0
  let unlimited = false
  let nextResetAt = 0
  for (const link of overview?.links || []) {
    const multiplierValue = Number(link.traffic_multiplier || 1)
    const multiplier = Number.isFinite(multiplierValue) && multiplierValue > 0 ? multiplierValue : 1
    upload += Math.max(0, Math.round(Number(link.traffic_upload_bytes || 0) * multiplier))
    download += Math.max(0, Math.round(Number(link.traffic_download_bytes || 0) * multiplier))
    counted += Math.max(0, Number(link.traffic_used_bytes || 0))
    if (Number(link.traffic_limit_bytes || 0) <= 0) unlimited = true
    else quota += Number(link.traffic_limit_bytes || 0)
    if (Number(link.traffic_reset_at || 0) > 0 && (!nextResetAt || Number(link.traffic_reset_at) < nextResetAt)) nextResetAt = Number(link.traffic_reset_at)
  }
  return {
    generated_at: overview?.generated_at || new Date().toISOString(),
    range_start: '',
    range_end: '',
    upload_bytes: upload,
    download_bytes: download,
    raw_total_bytes: upload + download,
    counted_total_bytes: counted,
    quota_bytes: quota,
    remaining_bytes: unlimited ? 0 : Math.max(0, quota - counted),
    next_reset_at: nextResetAt || undefined,
    unlimited,
    daily: [],
  }
}

function CustomerAnnouncementBar(props: {
  announcements: NonNullable<CustomerOverviewResponse['announcements']>
  onOpen: (index: number) => void
}) {
  if (!props.announcements.length) return null
  return (
    <Card size="small" className="customer-announcement-bar" title="公告栏">
      <Space wrap>
        {props.announcements.map((announcement, index) => (
          <Button key={announcement.id} type="link" onClick={() => props.onOpen(index)}>
            {announcement.title}
          </Button>
        ))}
      </Space>
    </Card>
  )
}

function customerAnnouncementReadKey(username: string): string {
  return `bridge-core.customer-announcements-read:${username}`
}

function customerAnnouncementIcon(level: string) {
  switch (level) {
    case 'success':
      return <CheckCircleOutlined />
    case 'warning':
      return <WarningOutlined />
    case 'error':
      return <CloseCircleOutlined />
    default:
      return <InfoCircleOutlined />
  }
}

function safeCustomerAnnouncementURL(value?: string): string {
  const raw = (value || '').trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    return ['http:', 'https:', 'tg:', 'mailto:', 'tel:'].includes(parsed.protocol) ? parsed.toString() : ''
  } catch {
    return ''
  }
}

function CustomerTopologyMap({ steps }: { steps: CustomerLinkStep[] }) {
  const visibleSteps = steps.length ? steps : [{ role: 'entry', label: '入口' }]
  return (
    <div className="customer-topology-canvas" aria-label="授权链路拓扑图">
      <div className="customer-topology-track">
        {visibleSteps.map((step, index) => {
          const displayLabel = customerTopologyStepLabel(step)
          const showExitIP = step.role === 'exit' && Boolean(step.exit_ip)
          return (
            <div key={`${step.role}-${step.label}-${index}`} className="customer-topology-segment">
              <div className={`customer-topology-node customer-topology-node-${step.role}`}>
                <span className="customer-topology-node-icon">
                  {topologyNodeIcon(step.role)}
                  {step.role === 'exit' ? <span className="customer-topology-node-flag">{countryFlag(step.country_code)}</span> : null}
                </span>
                <span className="customer-topology-node-label">{displayLabel}</span>
                {showExitIP ? <span className="customer-topology-node-meta">{step.exit_ip}</span> : null}
              </div>
              {index < visibleSteps.length - 1 ? (
                <div className="customer-topology-edge">
                  <span />
                </div>
              ) : null}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function customerTopologyStepLabel(step: CustomerLinkStep): string {
  if (step.role !== 'exit' || !step.exit_ip) {
    return step.label
  }
  const escapedIP = step.exit_ip.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return step.label.replace(new RegExp(`\\s*${escapedIP}\\s*$`), '').trim() || step.label
}

function topologyNodeIcon(role: string) {
  switch (role) {
    case 'entry':
      return '入口'
    case 'relay':
      return '转发'
    case 'exit':
      return '出口'
    default:
      return '节点'
  }
}

function customerLinkDisplayName(link: CustomerLinkView, draft?: string): string {
  const draftName = (draft || '').trim()
  const savedRemark = (link.remark || '').trim()
  const routeName = customerLinkRouteName(link)
  return draftName || savedRemark || routeName || link.client_email || '授权链路'
}

function customerLinkSummaryText(link: CustomerLinkView): string {
  const summary = (link.summary || '').trim()
  const withoutExit = summary.replace(/\s+出口\b.*$/u, '').trim()
  return withoutExit || customerLinkRouteName(link) || link.entry_client_name || `链路 #${link.assignment_id}`
}

function customerLinkRouteName(link: CustomerLinkView): string {
  const parts = [
    link.entry_client_name,
    ...(link.steps || []).filter((step) => step.role === 'relay').map((step) => step.label),
  ]
    .map((value) => (value || '').trim())
    .filter(Boolean)
  return Array.from(new Set(parts)).join('-')
}

function CustomerLinkTraffic({ link }: { link: CustomerLinkView }) {
  const trafficUsed = Math.max(0, Number(link.traffic_used_bytes || 0))
  const trafficLimit = Math.max(0, Number(link.traffic_limit_bytes || 0))
  const multiplierValue = Number(link.traffic_multiplier || 1)
  const multiplier = Number.isFinite(multiplierValue) && multiplierValue > 0 ? multiplierValue : 1
  const upload = Math.max(0, Math.round(Number(link.traffic_upload_bytes || 0) * multiplier))
  const download = Math.max(0, Math.round(Number(link.traffic_download_bytes || 0) * multiplier))
  const unlimited = trafficLimit <= 0
  const percent = unlimited ? 0 : Math.min(100, (trafficUsed / trafficLimit) * 100)
  const strokeColor = percent >= 90 ? '#ef4444' : percent >= 75 ? '#f59e0b' : '#2563eb'

  return (
    <div className="customer-link-traffic" aria-label="链路流量用量">
      <div className="customer-link-traffic-head">
        <Text strong>流量用量</Text>
        <Text className="customer-link-traffic-value">
          {formatBytes(trafficUsed)} / {unlimited ? '无上限' : formatBytes(trafficLimit)}
        </Text>
      </div>
      {unlimited ? (
        <div className="customer-link-traffic-unlimited" aria-label="流量无上限">
          <span className="customer-link-traffic-track"><span className="customer-link-traffic-fill" /></span>
          <Text type="secondary">无上限</Text>
        </div>
      ) : (
        <Progress className="customer-link-traffic-progress" percent={percent} showInfo={false} strokeColor={strokeColor} />
      )}
      <div className="customer-link-traffic-detail">
        <span>上传 {formatBytes(upload)}</span>
        <span>下载 {formatBytes(download)}</span>
      </div>
    </div>
  )
}

type CustomerLinkMetaItem = {
  key: string
  text: string
  expiry?: {
    endTime: number
    label: string
    percent?: number
    strokeColor: string
    source: 'xray' | 'billing'
  }
}

function customerLinkMetaItems(link: CustomerLinkView): CustomerLinkMetaItem[] {
  const items: CustomerLinkMetaItem[] = []
  if (Number(link.traffic_reset_at || 0) > 0) {
    items.push({
      key: 'traffic-reset',
      text: `下次流量重置时间：${formatCustomerResetTime(Number(link.traffic_reset_at || 0))}`,
    })
  }
  if (Number(link.revenue_amount || 0) > 0) {
    items.push({
      key: 'revenue',
      text: `费用：${formatCustomerRecurringPrice(Number(link.revenue_amount || 0), link.revenue_currency || 'CNY', link.revenue_cycle)}`,
    })
  }
  if (Number(link.xray_expire_time || 0) > 0 || Number(link.expire_time || 0) > 0 || (Number(link.start_time || 0) > 0 && customerBillingCycleMonths(link.revenue_cycle || link.expire_cycle) > 0)) {
    const expiry = customerLinkExpiryProgress(link)
    const xrayExpiryTime = Number(link.xray_expire_time || 0)
    const expiryLabel = expiry?.endTime || Number(link.expire_time || 0)
    items.push({
      key: 'expiry',
      text: `${xrayExpiryTime > 0 ? 'Xray 到期' : '客户端到期'}：${formatCustomerExpiryTime(expiryLabel)}`,
      expiry,
    })
  }
  return items
}

function customerLinkExpiryProgress(link: CustomerLinkView): CustomerLinkMetaItem['expiry'] {
  const xrayExpireTime = Number(link.xray_expire_time || 0)
  const expireTime = Number(link.expire_time || 0)
  const now = Date.now()
  const period = customerBillingPeriod(link, now)
  const hasXrayExpiry = Number.isFinite(xrayExpireTime) && xrayExpireTime > 0
  const hasBillingExpiry = Number.isFinite(expireTime) && expireTime > 0
  if (!hasXrayExpiry && !hasBillingExpiry && !period) return undefined
  // Xray owns the actual client validity. Billing periods still provide the
  // reset/progress baseline, but can never extend access past Xray expiry.
  const effectiveExpireTime = hasXrayExpiry ? xrayExpireTime : (period?.endTime ?? expireTime)
  const remainingDays = Math.ceil((effectiveExpireTime - now) / 86400000)
  const label = remainingDays <= 0 ? '已到期' : `剩余 ${remainingDays} 天`
  const progressEndTime = period && hasXrayExpiry
    ? Math.min(period.endTime, xrayExpireTime)
    : period?.endTime
  const percent = period && progressEndTime && progressEndTime > period.startTime
    ? Math.min(100, Math.max(0, (now - period.startTime) / (progressEndTime - period.startTime) * 100))
    : period
      ? 100
      : undefined

  return {
    endTime: effectiveExpireTime,
    label,
    percent,
    strokeColor: remainingDays <= 0 ? '#ef4444' : remainingDays <= 7 ? '#f59e0b' : '#2563eb',
    source: hasXrayExpiry ? 'xray' : 'billing',
  }
}

function customerBillingPeriod(link: CustomerLinkView, now: number): { startTime: number; endTime: number } | undefined {
  const startTime = Number(link.start_time || 0)
  const cycleMonths = customerBillingCycleMonths(link.revenue_cycle || link.expire_cycle)
  if (!Number.isFinite(startTime) || startTime <= 0 || cycleMonths <= 0) return undefined

  const anchor = new Date(startTime)
  const current = new Date(now)
  let cycleIndex = Math.max(0, Math.floor(monthDifference(anchor, current) / cycleMonths))
  let periodStart = addCalendarMonths(anchor, cycleIndex * cycleMonths)
  let nextPeriodStart = addCalendarMonths(anchor, (cycleIndex + 1) * cycleMonths)
  while (nextPeriodStart.getTime() <= now) {
    cycleIndex += 1
    periodStart = nextPeriodStart
    nextPeriodStart = addCalendarMonths(anchor, (cycleIndex + 1) * cycleMonths)
  }
  while (periodStart.getTime() > now && cycleIndex > 0) {
    cycleIndex -= 1
    nextPeriodStart = periodStart
    periodStart = addCalendarMonths(anchor, cycleIndex * cycleMonths)
  }

  return {
    startTime: periodStart.getTime(),
    // The next period starts at 00:00, so the current billing period ends just before it.
    endTime: nextPeriodStart.getTime() - 1,
  }
}

function customerBillingCycleMonths(cycle?: string): number {
  switch ((cycle || '').toLowerCase()) {
    case 'quarter':
    case 'quarterly':
      return 3
    case 'semiannual':
    case 'halfyear':
    case 'half-year':
      return 6
    case 'year':
    case 'yearly':
      return 12
    case 'month':
    case 'monthly':
      return 1
    default:
      return 0
  }
}

function monthDifference(start: Date, end: Date): number {
  return (end.getFullYear() - start.getFullYear()) * 12 + end.getMonth() - start.getMonth()
}

function addCalendarMonths(value: Date, months: number): Date {
  const result = new Date(value)
  const day = result.getDate()
  result.setDate(1)
  result.setMonth(result.getMonth() + months)
  const lastDay = new Date(result.getFullYear(), result.getMonth() + 1, 0).getDate()
  result.setDate(Math.min(day, lastDay))
  return result
}

function formatCustomerRecurringPrice(amount: number, currency: string, cycle?: string): string {
  return `${formatCustomerMoney(amount, currency)}/${cycleUnitLabel(cycle)}`
}

function formatCustomerMoney(amount: number, currency: string): string {
  const normalizedCurrency = (currency || 'CNY').toUpperCase()
  const amountText = formatCompactAmount(amount)
  if (normalizedCurrency === 'CNY') {
    return `${amountText}元`
  }
  return `${amountText}${normalizedCurrency}`
}

function formatCompactAmount(amount: number): string {
  if (Number.isInteger(amount)) {
    return String(amount)
  }
  try {
    return new Intl.NumberFormat('zh-CN', {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(amount)
  } catch {
    return amount.toFixed(2).replace(/\.?0+$/, '')
  }
}

function formatCustomerExpiryTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(value))
}

function formatCustomerResetTime(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function formatCustomerTrafficRecordTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}

function cycleUnitLabel(cycle?: string): string {
  switch (cycle) {
    case 'quarter':
      return '季'
    case 'semiannual':
      return '半年'
    case 'year':
      return '年'
    case 'month':
    default:
      return '月'
  }
}

function customerLinkImportURL(link: CustomerLinkView, draft?: string): string {
  const source = link.import_url || ''
  if (!source) {
    return ''
  }
  const displayName = customerLinkDisplayName(link, draft)
  if (source.toLowerCase().startsWith('vmess://')) {
    return rewriteVMessImportName(source, displayName)
  }
  return rewriteURLFragment(source, displayName)
}

function rewriteURLFragment(source: string, displayName: string): string {
  try {
    const url = new URL(source)
    url.hash = displayName
    return url.toString()
  } catch {
    const encodedName = encodeURIComponent(displayName)
    return source.includes('#') ? source.replace(/#.*$/, `#${encodedName}`) : `${source}#${encodedName}`
  }
}

function rewriteVMessImportName(source: string, displayName: string): string {
  try {
    const payload = source.slice('vmess://'.length).trim()
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const raw = decodeURIComponent(escape(window.atob(padded)))
    const data = JSON.parse(raw) as Record<string, unknown>
    data.ps = displayName
    const nextRaw = unescape(encodeURIComponent(JSON.stringify(data)))
    return `vmess://${window.btoa(nextRaw)}`
  } catch {
    return source
  }
}
