import { useEffect, useMemo, useState } from 'react'
import { Alert, App as AntdApp, Button, Card, Empty, Input, Space, Spin, Statistic, Tag, Typography } from 'antd'
import { CopyOutlined, LogoutOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'

import type { CustomerAuthResponse, CustomerLinkView, CustomerOverviewResponse, CustomerUser } from '../types'
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
  const [user, setUser] = useState<CustomerUser | null>(null)
  const [overview, setOverview] = useState<CustomerOverviewResponse | null>(null)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [remarkDrafts, setRemarkDrafts] = useState<Record<number, string>>({})

  useEffect(() => {
    void loadSession()
  }, [])

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

  async function copyImportURL(link: CustomerLinkView) {
    if (!link.import_url) {
      message.warning('当前链路没有可复制的客户端信息')
      return
    }
    try {
      await navigator.clipboard.writeText(link.import_url)
      message.success('客户端信息已复制')
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
              这里只展示已分配给你的链路、出口国家和出口 IP。内部转发 IP 与管理配置不会在客户侧暴露。
            </Paragraph>
          </div>
          <Space wrap>
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

                <div className="customer-chain-line">
                  {link.steps.map((step, index) => (
                    <span key={`${step.role}-${step.label}-${index}`} className={`customer-chain-step customer-chain-step-${step.role}`}>
                      {step.label}
                    </span>
                  ))}
                </div>

                <div className="customer-link-grid">
                  <div>
                    <Text type="secondary">客户客户端信息</Text>
                    <Input.TextArea value={link.import_url || '当前链路暂无可复制的客户端信息'} readOnly autoSize={{ minRows: 2, maxRows: 4 }} />
                    <Button style={{ marginTop: 8 }} type="primary" icon={<CopyOutlined />} disabled={!link.import_url} onClick={() => void copyImportURL(link)}>
                      复制客户端信息
                    </Button>
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
    </div>
  )
}
