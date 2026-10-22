import { adminEntryPath } from '../lib/appBasePath'
import type { AdminPageKey } from '../lib/adminRoute'
import type { ReactNode } from 'react'

export type AdminMenuKey = AdminPageKey | 'topology'
export type AdminMenuAction =
  | 'onOpenWorkbench'
  | 'onOpenAssets'
  | 'onOpenFinance'
  | 'onOpenTopology'
  | 'onOpenCustomers'
  | 'onOpenFrontProxies'
  | 'onOpenSupport'
  | 'onOpenAccessLogs'
  | 'onOpenAnnouncements'
  | 'onOpenSchedules'

export const adminMenu = [
  {
    key: 'dashboard',
    title: '工作台',
    description: '查看系统健康、实时流量与待处理事项',
    group: '监控',
    action: 'onOpenWorkbench',
  },
  {
    key: 'assets',
    title: 'Client 资产',
    description: '集中查看服务器状态、资源与节点配置',
    group: '监控',
    action: 'onOpenAssets',
  },
  {
    key: 'topology',
    title: '链路拓扑',
    description: '追踪入口、中转与出口之间的连接',
    group: '监控',
    action: 'onOpenTopology',
  },
  {
    key: 'customers',
    title: '用户管理',
    description: '管理账号、订阅与节点授权',
    group: '业务',
    action: 'onOpenCustomers',
  },
  {
    key: 'finance',
    title: '财务总览',
    description: '查看收入、成本与收费周期',
    group: '业务',
    action: 'onOpenFinance',
    adminOnly: true,
  },
  {
    key: 'front-proxies',
    title: '前置代理',
    description: '管理订阅转发组与公共出口',
    group: '业务',
    action: 'onOpenFrontProxies',
    frontProxyOnly: true,
  },
  { key: 'support', title: '在线客服', description: '处理客户会话与留言', group: '业务', action: 'onOpenSupport' },
  {
    key: 'access-logs',
    title: '访问日志',
    description: '检索访问记录，排查连接问题',
    group: '运维',
    action: 'onOpenAccessLogs',
    adminOnly: true,
  },
  {
    key: 'schedules',
    title: '定时任务',
    description: '设置任务时间与执行频率',
    group: '运维',
    action: 'onOpenSchedules',
    adminOnly: true,
  },
  {
    key: 'settings',
    title: '客户公告',
    description: '管理公告内容与发布状态',
    group: '系统',
    action: 'onOpenAnnouncements',
    adminOnly: true,
  },
] satisfies Array<{
  key: AdminMenuKey
  title: string
  description: string
  group: string
  action: AdminMenuAction
  adminOnly?: boolean
  frontProxyOnly?: boolean
}>

export function visibleAdminMenu(canManageSystem: boolean, isAreaManagerAccount: boolean, isOperatorAccount = false) {
  return visibleAdminMenuForRole(canManageSystem, isAreaManagerAccount, isOperatorAccount)
}

function visibleAdminMenuForRole(canManageSystem: boolean, isAreaManagerAccount: boolean, isOperatorAccount: boolean) {
  return adminMenu.filter(
    (item) =>
      (!isOperatorAccount || item.key === 'customers' || item.key === 'support') &&
      (!('adminOnly' in item) || canManageSystem) &&
      (!('frontProxyOnly' in item) || canManageSystem || isAreaManagerAccount),
  )
}

/** ProLayout filters pathless groups before converting legacy `routes` to children. */
export function adminMenuRoutes(
  canManageSystem: boolean,
  isAreaManagerAccount: boolean,
  isOperatorAccount = false,
  iconFor?: (key: AdminMenuKey) => ReactNode,
) {
  const menu = visibleAdminMenuForRole(canManageSystem, isAreaManagerAccount, isOperatorAccount)
  return [...new Set(menu.map((item) => item.group))].map((group) => ({
    name: group,
    key: group,
    children: menu
      .filter((item) => item.group === group)
      .map((item) => ({ path: `/${item.key}`, name: item.title, icon: iconFor?.(item.key) })),
  }))
}

export function currentAdminMenu(page: AdminPageKey, topology: boolean) {
  return adminMenu.find((item) => item.key === (page === 'dashboard' && topology ? 'topology' : page)) || adminMenu[0]
}

/** Keep deployment prefixes and existing query-style bookmarks intact. */
export function adminMenuHref(key: AdminMenuKey, pathname?: string): string {
  const entry = adminEntryPath(pathname)
  return key === 'dashboard' ? entry : `${entry}?page=${key}`
}
