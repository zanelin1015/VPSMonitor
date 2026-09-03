import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Checkbox, Empty, Input, Modal, QRCode, Space, Spin, Statistic, Tag, Typography } from 'antd'
import { BgColorsOutlined, CheckCircleOutlined, CheckOutlined, CloseCircleOutlined, CloseOutlined, CopyOutlined, EditOutlined, InfoCircleOutlined, LockOutlined, LogoutOutlined, QrcodeOutlined, ReloadOutlined, WarningOutlined } from '@ant-design/icons'

import type { CustomerAuthResponse, CustomerLinkStep, CustomerLinkView, CustomerOverviewResponse, CustomerUser } from '../types'
import { countryFlag, fetchJSON, formatDateTime } from '../lib/appHelpers'
import { formatBytes } from '../lib/traffic'
import { LoginScreen } from './LoginScreen'
import { CustomerSupportWidget } from './CustomerSupportWidget'
import { clearCustomFrontendCode } from './VisualEffects'

const { Paragraph, Text, Title } = Typography

export function CustomerPortal() {
  const { message } = AntdApp.useApp()
  const [sessionLoading, setSessionLoading] = useState(true)
  const [loginLoading, setLoginLoading] = useState(false)
  const [overviewLoading, setOverviewLoading] = useState(false)
  const [savingRemarkID, setSavingRemarkID] = useState<number | null>(null)
  const [styleModalOpen, setStyleModalOpen] = useState(false)
  const [styleSaving, setStyleSaving] = useState(false)
  const [styleDraft, setStyleDraft] = useState('')
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
  const [editingRemarkID, setEditingRemarkID] = useState<number | null>(null)
  const [user, setUser] = useState<CustomerUser | null>(null)
  const [overview, setOverview] = useState<CustomerOverviewResponse | null>(null)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [remarkDrafts, setRemarkDrafts] = useState<Record<number, string>>({})
  const lastAnnouncementSetRef = useRef('')

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
    applyCustomerStyle(user?.style_code || '')
    setStyleDraft(user?.style_code || '')
    return () => applyCustomerStyle('')
  }, [user?.style_code])

  useEffect(() => {
    const announcements = overview?.announcements || []
    const setKey = JSON.stringify(announcements.map((item) => [item.id, item.level, item.title, item.content, item.link_label, item.link_url]))
    if (announcements.length === 0) {
      setAnnouncementModalOpen(false)
      lastAnnouncementSetRef.current = ''
      return
    }
    if (setKey !== lastAnnouncementSetRef.current) {
      lastAnnouncementSetRef.current = setKey
      setAnnouncementIndex(0)
      setAnnouncementModalOpen(true)
    }
  }, [overview?.announcements])

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
    setRemarkDrafts({})
    setAnnouncementModalOpen(false)
    setAnnouncementIndex(0)
    lastAnnouncementSetRef.current = ''
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

  async function saveCustomerStyle() {
    setStyleSaving(true)
    try {
      const data = await fetchJSON<CustomerAuthResponse>('/api/v1/customer/style', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ style_code: styleDraft }),
      })
      setUser(data.user)
      setStyleModalOpen(false)
      message.success(styleDraft.trim() ? '页面样式已保存' : '已恢复默认页面样式')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存页面样式失败')
    } finally {
      setStyleSaving(false)
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
        title="ZaneLin授权链路面板"
        subtitle=""
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
            <Button shape="circle" icon={<BgColorsOutlined />} onClick={() => {
              setStyleDraft(user.style_code || '')
              setStyleModalOpen(true)
            }} />
            <Button shape="circle" icon={<CopyOutlined />} title="复制 Clash/Mihomo 订阅" onClick={openClashSubscriptionModal} />
            <Button shape="circle" icon={<LockOutlined />} onClick={openPasswordModal} />
            <Button shape="circle" icon={<ReloadOutlined />} loading={overviewLoading} onClick={() => void loadOverview()} />
            <Button shape="circle" icon={<LogoutOutlined />} onClick={() => void logout()} />
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
          <Space wrap>
            <Button icon={<BgColorsOutlined />} onClick={() => {
              setStyleDraft(user.style_code || '')
              setStyleModalOpen(true)
            }}>页面样式</Button>
            <Button icon={<CopyOutlined />} onClick={openClashSubscriptionModal}>复制 Clash/Mihomo 订阅</Button>
            <Button icon={<LockOutlined />} onClick={openPasswordModal}>修改密码</Button>
            <Button icon={<ReloadOutlined />} loading={overviewLoading} onClick={() => void loadOverview()}>刷新</Button>
            <Button icon={<LogoutOutlined />} onClick={() => void logout()}>退出</Button>
          </Space>
        </header>
        <Alert
          className="customer-policy-alert"
          type="info"
          showIcon
          message="授权使用规则"
          description="仅限已授权用户使用，可按独享或共享方式开放；禁止滥发、攻击、诈骗、爬虫滥用、扫描爆破等高风险行为。发现异常时，管理员可随时停用账号或授权链路，并停用对应 x-ui client。"
        />

        <div className="customer-board-layout">
          <aside className="customer-board-side">
            <Card bordered={false} className="surface-card customer-board-profile">
              <Text type="secondary">用户账号</Text>
              <Title level={3}>{user.display_name || user.username}</Title>
              <Tag color="blue">登录可见</Tag>
              <Button block icon={<CopyOutlined />} onClick={openClashSubscriptionModal}>
                复制 Clash/Mihomo 订阅
              </Button>
              <Text type="secondary">数据更新时间</Text>
              <Text>{overview?.generated_at ? formatDateTime(overview.generated_at) : '-'}</Text>
            </Card>
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
                    <Paragraph className="customer-link-summary">{link.summary}</Paragraph>
                  </div>
                  {billingItems.length ? (
                    <div className="customer-link-meta-row" aria-label="授权链路费用和过期时间">
                      {billingItems.map((item) => (
                        <span key={item.key} className={`customer-link-meta-item customer-link-meta-${item.key}`}>{item.text}</span>
                      ))}
                    </div>
                  ) : null}
                  <Space wrap className="customer-link-exit-tags">
                    {link.exit_country_code || link.exit_country_name ? <Tag color="blue">出口 {countryFlag(link.exit_country_code)} {link.exit_country_code || link.exit_country_name}</Tag> : null}
                    {link.exit_ip ? <Tag color="geekblue">{link.exit_ip}</Tag> : null}
                    {!link.resolved ? <Tag color="orange">待解析</Tag> : null}
                  </Space>
                </div>

                <div className="customer-link-visual-row">
                  <div className="customer-topology-scroll">
                    <CustomerTopologyMap steps={link.steps} />
                  </div>
                  <aside className="customer-qr-inline">
                    <div className="customer-qr-actions">
                      <Button type="primary" icon={<CopyOutlined />} disabled={!effectiveImportURL} onClick={() => void copyImportURL(link)}>
                        复制链接
                      </Button>
                      <Button icon={<QrcodeOutlined />} disabled={!effectiveImportURL} onClick={() => setQrLink(link)}>
                        放大二维码
                      </Button>
                    </div>
                    {effectiveImportURL ? (
                      <button type="button" className="customer-qr-button" onClick={() => setQrLink(link)}>
                        <QRCode value={effectiveImportURL} bordered={false} size={132} />
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
          </section>
        </div>
      </div>

      <CustomerAnnouncementModal
        announcements={overview?.announcements || []}
        index={announcementIndex}
        open={announcementModalOpen}
        onIndexChange={setAnnouncementIndex}
        onClose={() => setAnnouncementModalOpen(false)}
      />

      <CustomerSupportWidget />

      <Modal
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
                    <div className="muted-line">{link.summary}{link.resolved && link.import_url ? '' : '（暂不可导出）'}</div>
                  </div>
                </Checkbox>
              ))}
            </Space>
          </Checkbox.Group>
          {!(overview?.links || []).length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无授权链路" /> : null}
        </Space>
      </Modal>

      <Modal
        title="我的页面样式"
        open={styleModalOpen}
        onCancel={() => setStyleModalOpen(false)}
        width={820}
        footer={[
          <Button key="reset" onClick={() => setStyleDraft('')}>恢复默认</Button>,
          <Button key="cancel" onClick={() => setStyleModalOpen(false)}>取消</Button>,
          <Button key="save" type="primary" loading={styleSaving} onClick={() => void saveCustomerStyle()}>保存样式</Button>,
        ]}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="只影响你自己的用户页面"
            description="可以填写 CSS，或完整的 <style> / <script> 片段；留空则使用系统默认页面样式。"
          />
          <Input.TextArea
            value={styleDraft}
            onChange={(event) => setStyleDraft(event.target.value)}
            autoSize={{ minRows: 12, maxRows: 22 }}
            placeholder={`<style>\n.customer-page-shell { --customer-card-radius: 28px; }\n.customer-hero { background: rgba(255,255,255,.88); }\n</style>`}
          />
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
            <QRCode value={customerLinkImportURL(qrLink, remarkDrafts[qrLink.assignment_id])} size={260} bordered={false} />
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
}) {
  const { announcements, index, open, onIndexChange, onClose } = props
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
        isLast ? <Button key="close" type="primary" onClick={onClose}>我知道了</Button> : null,
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
  return draftName || link.remark || link.entry_client_name || link.client_email || '授权链路'
}

function customerLinkMetaItems(link: CustomerLinkView): Array<{ key: string; text: string }> {
  const items: Array<{ key: string; text: string }> = []
  const trafficUsed = Math.max(0, Number(link.traffic_used_bytes || 0))
  const trafficLimit = Math.max(0, Number(link.traffic_limit_bytes || 0))
  if (trafficUsed > 0 || trafficLimit > 0) {
    items.push({
      key: 'traffic',
      text: `流量：已用 ${formatBytes(trafficUsed)} / ${trafficLimit > 0 ? `双向限额 ${formatBytes(trafficLimit)}` : '无上限'}`,
    })
  }
  if (Number(link.revenue_amount || 0) > 0) {
    items.push({
      key: 'revenue',
      text: `费用：${formatCustomerRecurringPrice(Number(link.revenue_amount || 0), link.revenue_currency || 'CNY', link.revenue_cycle)}`,
    })
  }
  if (Number(link.expire_time || 0) > 0) {
    items.push({
      key: 'expiry',
      text: `客户端到期：${formatCustomerExpiryTime(Number(link.expire_time || 0))}`,
    })
  }
  return items
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

function applyCustomerStyle(styleCode: string) {
  document.querySelectorAll('[data-vpsmonitor-customer-style="true"]').forEach((node) => node.remove())
  const code = styleCode.trim()
  if (!code) {
    return
  }
  const template = document.createElement('template')
  template.innerHTML = code.includes('<') ? code : `<style>${code}</style>`
  Array.from(template.content.childNodes).forEach((node) => appendCustomerStyleNode(node))
}

function appendCustomerStyleNode(node: Node) {
  if (node.nodeType === Node.TEXT_NODE && !node.textContent?.trim()) {
    return
  }
  if (node.nodeName.toLowerCase() === 'script') {
    const source = node as HTMLScriptElement
    const script = document.createElement('script')
    Array.from(source.attributes).forEach((attr) => script.setAttribute(attr.name, attr.value))
    script.text = source.text
    script.dataset.vpsmonitorCustomerStyle = 'true'
    script.async = false
    document.body.appendChild(script)
    return
  }
  const element = node.cloneNode(true) as HTMLElement
  if (element instanceof HTMLElement) {
    element.dataset.vpsmonitorCustomerStyle = 'true'
  }
  document.body.appendChild(element)
}
