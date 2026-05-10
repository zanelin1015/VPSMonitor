import { useEffect, useMemo, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Empty, Input, Modal, QRCode, Space, Spin, Statistic, Tag, Typography } from 'antd'
import { BgColorsOutlined, CopyOutlined, LogoutOutlined, QrcodeOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { CustomerAuthResponse, CustomerLinkStep, CustomerLinkView, CustomerOverviewResponse, CustomerUser } from '../types'
import { fetchJSON, formatDateTime } from '../lib/appHelpers'
import { LoginScreen } from './LoginScreen'
import { VisualEffects } from './VisualEffects'

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
  const [qrLink, setQrLink] = useState<CustomerLinkView | null>(null)
  const [user, setUser] = useState<CustomerUser | null>(null)
  const [overview, setOverview] = useState<CustomerOverviewResponse | null>(null)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [remarkDrafts, setRemarkDrafts] = useState<Record<number, string>>({})

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

  async function copyImportURL(link: CustomerLinkView) {
    if (!link.import_url) {
      message.warning('当前链路没有可复制的客户端链接')
      return
    }
    try {
      await navigator.clipboard.writeText(link.import_url)
      message.success('客户端链接已复制')
    } catch {
      message.error('复制失败，请手动复制')
    }
  }

  if (sessionLoading) {
    return (
      <>
        <VisualEffects />
        <div className="login-shell">
          <Spin size="large" />
        </div>
      </>
    )
  }

  if (!user) {
    return (
      <>
        <VisualEffects />
        <LoginScreen
          title="南风客户中心"
          subtitle="客户登录"
          loginForm={loginForm}
          loginLoading={loginLoading}
          onChange={setLoginForm}
          onLogin={login}
        />
      </>
    )
  }

  return (
    <div className="page-shell customer-page-shell">
      <VisualEffects />
      <div className="page-background page-background-left" />
      <div className="page-background page-background-right" />
      <div className="customer-shell">
        <header className="customer-hero">
          <div>
            <div className="eyebrow">客户链路中心</div>
            <Title level={1}>{user.display_name || user.username}</Title>
            <Paragraph className="hero-copy">
              这里只展示已分配给你的拓扑图、出口国家和出口 IP。内部转发 IP 与管理配置不会在客户侧暴露。
            </Paragraph>
          </div>
          <Space wrap>
            <Button icon={<BgColorsOutlined />} onClick={() => {
              setStyleDraft(user.style_code || '')
              setStyleModalOpen(true)
            }}>页面样式</Button>
            <Button icon={<ReloadOutlined />} loading={overviewLoading} onClick={() => void loadOverview()}>刷新</Button>
            <Button icon={<LogoutOutlined />} onClick={() => void logout()}>退出</Button>
          </Space>
        </header>

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
            {(overview?.links || []).map((link) => (
              <Card key={link.assignment_id} bordered={false} className="surface-card customer-link-card">
                <div className="customer-link-head">
                  <div>
                    <Title level={3}>{link.entry_client_name}</Title>
                    <Paragraph className="customer-link-summary">{link.summary}</Paragraph>
                  </div>
                  <Space wrap>
                    {link.exit_country_code || link.exit_country_name ? <Tag color="blue">出口 {link.exit_country_code || link.exit_country_name}</Tag> : null}
                    {link.exit_ip ? <Tag color="geekblue">{link.exit_ip}</Tag> : null}
                    {!link.resolved ? <Tag color="orange">待解析</Tag> : null}
                  </Space>
                </div>

                {link.unresolved_reason ? <Alert type="warning" showIcon message="链路解析提示" description={link.unresolved_reason} /> : null}

                <CustomerTopologyMap steps={link.steps} />

                <div className="customer-link-grid customer-link-grid-with-qr">
                  <div>
                    <Text type="secondary">客户客户端链接</Text>
                    <Input.TextArea value={link.import_url || '当前链路暂无可复制的客户端链接'} readOnly autoSize={{ minRows: 2, maxRows: 4 }} />
                    <Space wrap style={{ marginTop: 8 }}>
                      <Button type="primary" icon={<CopyOutlined />} disabled={!link.import_url} onClick={() => void copyImportURL(link)}>
                        复制链接
                      </Button>
                      <Button icon={<QrcodeOutlined />} disabled={!link.import_url} onClick={() => setQrLink(link)}>
                        查看二维码
                      </Button>
                    </Space>
                  </div>
                  <div className="customer-qr-inline">
                    <Text type="secondary">二维码</Text>
                    {link.import_url ? (
                      <button type="button" className="customer-qr-button" onClick={() => setQrLink(link)}>
                        <QRCode value={link.import_url} bordered={false} size={116} />
                        <span>点击放大</span>
                      </button>
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无二维码" />
                    )}
                  </div>
                  <div>
                    <Text type="secondary">我的备注</Text>
                    <Input.TextArea
                      placeholder="给这条链路添加备注，例如：主站备用入口"
                      value={remarkDrafts[link.assignment_id] || ''}
                      autoSize={{ minRows: 3, maxRows: 5 }}
                      maxLength={2000}
                      onChange={(event) => setRemarkDrafts((current) => ({ ...current, [link.assignment_id]: event.target.value }))}
                    />
                    <Button style={{ marginTop: 8 }} icon={<SaveOutlined />} loading={savingRemarkID === link.assignment_id} onClick={() => void saveRemark(link)}>
                      保存备注
                    </Button>
                  </div>
                </div>
              </Card>
            ))}
          </Space>
        </Spin>
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
            message="只影响你自己的客户页面"
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
        title={qrLink ? `${qrLink.entry_client_name} 二维码` : '客户端二维码'}
        open={Boolean(qrLink)}
        onCancel={() => setQrLink(null)}
        footer={[
          <Button key="close" onClick={() => setQrLink(null)}>关闭</Button>,
          <Button key="copy" type="primary" disabled={!qrLink?.import_url} onClick={() => qrLink && void copyImportURL(qrLink)}>复制链接</Button>,
        ]}
      >
        {qrLink?.import_url ? (
          <Space direction="vertical" size="middle" style={{ width: '100%', alignItems: 'center' }}>
            <QRCode value={qrLink.import_url} size={260} bordered={false} />
            <Input.TextArea value={qrLink.import_url} readOnly autoSize={{ minRows: 3, maxRows: 6 }} />
          </Space>
        ) : <Empty description="暂无二维码" />}
      </Modal>
    </div>
  )
}

function CustomerTopologyMap({ steps }: { steps: CustomerLinkStep[] }) {
  const visibleSteps = steps.length ? steps : [{ role: 'entry', label: '入口' }]
  return (
    <div className="customer-topology-canvas" aria-label="客户链路拓扑图">
      <div className="customer-topology-track">
        {visibleSteps.map((step, index) => (
          <div key={`${step.role}-${step.label}-${index}`} className="customer-topology-segment">
            <div className={`customer-topology-node customer-topology-node-${step.role}`}>
              <span className="customer-topology-node-icon">{topologyNodeIcon(step.role)}</span>
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
