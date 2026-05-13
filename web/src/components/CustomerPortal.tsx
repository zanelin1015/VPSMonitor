import { useEffect, useMemo, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Empty, Input, Modal, QRCode, Space, Spin, Statistic, Tag, Typography } from 'antd'
import { BgColorsOutlined, CheckOutlined, CloseOutlined, CopyOutlined, EditOutlined, LockOutlined, LogoutOutlined, QrcodeOutlined, ReloadOutlined } from '@ant-design/icons'

import type { CustomerAuthResponse, CustomerLinkStep, CustomerLinkView, CustomerOverviewResponse, CustomerUser } from '../types'
import { countryFlag, fetchJSON, formatDateTime } from '../lib/appHelpers'
import { LoginScreen } from './LoginScreen'
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
  const [passwordForm, setPasswordForm] = useState({
    current_password: '',
    new_password: '',
    confirm_password: '',
  })
  const [qrLink, setQrLink] = useState<CustomerLinkView | null>(null)
  const [editingRemarkID, setEditingRemarkID] = useState<number | null>(null)
  const [user, setUser] = useState<CustomerUser | null>(null)
  const [overview, setOverview] = useState<CustomerOverviewResponse | null>(null)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [remarkDrafts, setRemarkDrafts] = useState<Record<number, string>>({})

  useEffect(() => {
    clearCustomFrontendCode()
    return () => clearCustomFrontendCode()
  }, [])

  useEffect(() => {
    void loadSession()
  }, [])

  useEffect(() => {
    applyCustomerStyle(user?.style_code || '')
    setStyleDraft(user?.style_code || '')
    return () => applyCustomerStyle('')
  }, [user?.style_code])

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
        title="AlanStone链路面板"
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
            <div className="eyebrow">AlanStone链路面板</div>
            <Title level={2}>{user.display_name || user.username}</Title>
            <Text type="secondary">{overview?.generated_at ? formatDateTime(overview.generated_at) : '等待数据同步'}</Text>
          </div>
          <div className="customer-mobile-actions">
            <Button shape="circle" icon={<BgColorsOutlined />} onClick={() => {
              setStyleDraft(user.style_code || '')
              setStyleModalOpen(true)
            }} />
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
            <div className="eyebrow">用户看板 / 我的链路</div>
            <Title level={1}>{user.display_name || user.username}</Title>
          </div>
          <Space wrap>
            <Button icon={<BgColorsOutlined />} onClick={() => {
              setStyleDraft(user.style_code || '')
              setStyleModalOpen(true)
            }}>页面样式</Button>
            <Button icon={<LockOutlined />} onClick={openPasswordModal}>修改密码</Button>
            <Button icon={<ReloadOutlined />} loading={overviewLoading} onClick={() => void loadOverview()}>刷新</Button>
            <Button icon={<LogoutOutlined />} onClick={() => void logout()}>退出</Button>
          </Space>
        </header>

        <div className="customer-board-layout">
          <aside className="customer-board-side">
            <Card bordered={false} className="surface-card customer-board-profile">
              <Text type="secondary">用户账号</Text>
              <Title level={3}>{user.display_name || user.username}</Title>
              <Tag color="blue">登录可见</Tag>
              <Text type="secondary">数据更新时间</Text>
              <Text>{overview?.generated_at ? formatDateTime(overview.generated_at) : '-'}</Text>
            </Card>
            <Card bordered={false} className="surface-card customer-board-profile">
              <Text type="secondary">看板摘要</Text>
              <div className="customer-board-score">
                <strong>{overview?.links.length || 0}</strong>
                <span>条链路</span>
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
                <Statistic title="已分配链路" value={overview?.links.length || 0} suffix="条" />
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
                <Empty description="暂无分配链路，请联系管理员开通" />
              </Card>
            ) : null}
            {(overview?.links || []).map((link) => {
              const effectiveName = customerLinkDisplayName(link, remarkDrafts[link.assignment_id])
              const effectiveImportURL = customerLinkImportURL(link, remarkDrafts[link.assignment_id])
              return (
              <Card key={link.assignment_id} bordered={false} className="surface-card customer-link-card">
                <div className="customer-link-head">
                  <div>
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
                  <Space wrap>
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

function CustomerTopologyMap({ steps }: { steps: CustomerLinkStep[] }) {
  const visibleSteps = steps.length ? steps : [{ role: 'entry', label: '入口' }]
  return (
    <div className="customer-topology-canvas" aria-label="用户链路拓扑图">
      <div className="customer-topology-track">
        {visibleSteps.map((step, index) => (
          <div key={`${step.role}-${step.label}-${index}`} className="customer-topology-segment">
            <div className={`customer-topology-node customer-topology-node-${step.role}`}>
              <span className="customer-topology-node-icon">
                {topologyNodeIcon(step.role)}
                {step.role === 'exit' ? <span className="customer-topology-node-flag">{countryFlag(step.country_code)}</span> : null}
              </span>
              <span className="customer-topology-node-label">{step.label}</span>
              {step.role === 'exit' && step.exit_ip ? <span className="customer-topology-node-meta">{step.exit_ip}</span> : null}
            </div>
            {index < visibleSteps.length - 1 ? (
              <div className="customer-topology-edge">
                <span />
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  )
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
  return draftName || link.remark || link.entry_client_name || link.client_email || '用户链路'
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
