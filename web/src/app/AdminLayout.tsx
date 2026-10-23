import {
  ApartmentOutlined,
  ApiOutlined,
  BellOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  DollarOutlined,
  FileSearchOutlined,
  MessageOutlined,
  ReloadOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { PageContainer, ProLayout } from '@ant-design/pro-components'
import { Badge, Button, Space, Typography } from 'antd'
import { useState, type ReactNode } from 'react'
import { AdminHeaderActions, type AdminShellNavigationProps } from '../components/AdminShellNavigation'
import { adminMenuHref, adminMenuRoutes, currentAdminMenu, visibleAdminMenu } from './adminMenu'

const icons = {
  dashboard: <DashboardOutlined />,
  assets: <CloudServerOutlined />,
  topology: <ApartmentOutlined />,
  customers: <TeamOutlined />,
  finance: <DollarOutlined />,
  'front-proxies': <ApiOutlined />,
  support: <MessageOutlined />,
  'access-logs': <FileSearchOutlined />,
  schedules: <ReloadOutlined />,
  settings: <BellOutlined />,
}

export function AdminLayout({
  navigation,
  children,
  overlays,
}: {
  navigation: AdminShellNavigationProps
  children: ReactNode
  overlays?: ReactNode
}) {
  const [collapsed, setCollapsed] = useState(false)
  const current = currentAdminMenu(navigation.activeAdminPage, navigation.topologyVisible)
  const menu = visibleAdminMenu(
    navigation.canManageSystem,
    navigation.isAreaManagerAccount,
    navigation.isOperatorAccount,
  )
  const pathFor = (key: Parameters<typeof adminMenuHref>[0]) => adminMenuHref(key)

  return (
    <div className={`admin-pro-layout page-shell admin-page-shell${current.key === 'dashboard' ? ' admin-pro-workbench-layout' : ''}`}>
      <ProLayout
        title="ZaneLin"
        logo={
          <span className="admin-pro-logo" aria-hidden>
            Z
          </span>
        }
        layout="mix"
        siderWidth={216}
        collapsed={collapsed}
        onCollapse={setCollapsed}
        fixSiderbar
        fixedHeader
        location={{ pathname: `/${current.key}` }}
        route={{
          routes: adminMenuRoutes(
            navigation.canManageSystem,
            navigation.isAreaManagerAccount,
            navigation.isOperatorAccount,
            (key) =>
              key === 'support' ? (
                <Badge count={navigation.supportUnreadCount} size="small">
                  {icons[key]}
                </Badge>
              ) : (
                icons[key]
              ),
          ),
        }}
        menu={{ locale: false, type: 'group' }}
        menuItemRender={(item, dom) => (
          <a
            href={pathFor(menu.find((candidate) => `/${candidate.key}` === item.path)?.key || current.key)}
            onClick={(event) => {
              if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
              event.preventDefault()
              const target = menu.find((candidate) => `/${candidate.key}` === item.path)
              if (target) navigation[target.action]()
              if (item.isMobile) setCollapsed(true)
            }}
          >
            {dom}
          </a>
        )}
        actionsRender={() => AdminHeaderActions(navigation)}
        menuFooterRender={() =>
          collapsed ? null : (
            <div className="admin-pro-footer">
              <Badge status="success" text={`在线 ${navigation.onlineAgentCount}/${navigation.scopedAgentCount}`} />
              <Typography.Text type="secondary">Server {navigation.serverVersionLabel}</Typography.Text>
            </div>
          )
        }
        breadcrumbRender={false}
        contentStyle={{ padding: 0 }}
      >
        <PageContainer
          className="admin-pro-page"
          title={current.title}
          breadcrumbRender={false}
          content={<Typography.Text type="secondary">{current.description}</Typography.Text>}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} loading={navigation.agentsLoading} onClick={navigation.onRefreshAgents}>
                刷新
              </Button>
              {navigation.canManageSystem && current.key === 'assets' ? (
                <Button type="primary" icon={<DeploymentUnitOutlined />} onClick={navigation.onOpenClientInstall}>
                  安装 Client
                </Button>
              ) : null}
            </Space>
          }
        >
          <div className="admin-pro-page-body">{children}</div>
        </PageContainer>
      </ProLayout>
      {overlays}
    </div>
  )
}
