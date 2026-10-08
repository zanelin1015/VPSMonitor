import { useState, type ChangeEvent } from 'react'
import { Alert, Avatar, Button, Card, Col, Divider, Dropdown, Empty, Input, InputNumber, Modal, Popconfirm, QRCode, Row, Select, Space, Spin, Switch, Tabs, Tag, Typography } from 'antd'
import type { MenuProps } from 'antd'
import {
  BellOutlined,
  CloudDownloadOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  LogoutOutlined,
  PlusOutlined,
  SettingOutlined,
  TeamOutlined,
  UploadOutlined,
} from '@ant-design/icons'

import type { AdminUser, AgentListItem, ScheduledTaskSettings, SystemInfo, TelegramBot, UpdateLatestInfo, XUIClientView, XUINodeView, XUIOverview } from '../types'
import type {
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  FrontendSettingsForm,
  TelegramBotForm,
  XUIAddClientActionForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from '../lib/appHelpers'
import { XUI_ACTION_KINDS, clientInstallCommandByKind, defaultTelegramBotForm } from '../lib/appHelpers'
import { ClientInstallCommandBox } from './ClientInstallCommandBox'
import { AdminHelpHint } from './AdminHelpHint'
import { TelegramBotPanel } from './TelegramBotPanel'
import { renderAddClientActionForm, renderOutboundActionForm, renderRoutingActionForm } from './XUIActionForms'

const { Text } = Typography

export interface AccountFormState {
  current_password: string
  new_username: string
  new_password: string
  confirm_password: string
  avatar_url: string
}

export function PersonalCenterDropdown(props: {
  adminUser: AdminUser
  systemInfo?: SystemInfo | null
  canManageSystem?: boolean
  onOpenAccount: () => void
  onOpenClientInstall: () => void
  onOpenTelegram: () => void
  onOpenCustomers: () => void
  onOpenFrontendSettings: () => void
  onOpenUpdates: () => void
  onLogout: () => void
}) {
  const { adminUser, systemInfo, canManageSystem = true, onOpenAccount, onOpenClientInstall, onOpenTelegram, onOpenCustomers, onOpenFrontendSettings, onOpenUpdates, onLogout } = props
  const items: MenuProps['items'] = [
    {
      key: 'profile',
      disabled: true,
      label: (
        <div className="personal-center-menu-profile">
          <AdminAvatar user={adminUser} size={52} className="personal-center-menu-avatar" />
          <div>
            <Text type="secondary">{adminUser.role === 'area_manager' ? '当前区域账号' : '当前管理员'}</Text>
            <div className="personal-center-menu-name">{adminUser.username}</div>
            <Space wrap size={6}>
              <Tag color="success">已登录</Tag>
              {adminUser.role === 'area_manager' ? <Tag color="gold">区域管理</Tag> : null}
              {systemInfo?.version ? <Tag color="blue">Server v{systemInfo.version}</Tag> : null}
            </Space>
          </div>
        </div>
      ),
    },
    { type: 'divider' },
    ...(canManageSystem ? [{ key: 'account', icon: <EditOutlined />, label: '账号与头像' }] : []),
    ...(canManageSystem ? [{ key: 'client-install', icon: <CloudDownloadOutlined />, label: 'Client 安装命令' }] : []),
    ...(canManageSystem ? [{ key: 'telegram', icon: <BellOutlined />, label: 'TG 告警机器人' }] : []),
    { key: 'customers', icon: <TeamOutlined />, label: '人员管理' },
    ...(canManageSystem ? [{ key: 'frontend', icon: <SettingOutlined />, label: '前端样式自定义' }] : []),
    ...(canManageSystem ? [{ key: 'updates', icon: <SettingOutlined />, label: '在线升级' }] : []),
    { type: 'divider' },
    { key: 'logout', danger: true, icon: <LogoutOutlined />, label: '退出登录' },
  ]
  const onMenuClick: MenuProps['onClick'] = ({ key }) => {
    switch (key) {
      case 'account':
        onOpenAccount()
        break
      case 'client-install':
        onOpenClientInstall()
        break
      case 'telegram':
        onOpenTelegram()
        break
      case 'customers':
        onOpenCustomers()
        break
      case 'frontend':
        onOpenFrontendSettings()
        break
      case 'updates':
        onOpenUpdates()
        break
      case 'logout':
        onLogout()
        break
    }
  }

  return (
    <Dropdown menu={{ items, onClick: onMenuClick }} trigger={['click']} placement="bottomRight" overlayClassName="personal-center-dropdown">
      <Button className="personal-center-button" aria-label="个人中心" title="个人中心" onClick={(event) => event.preventDefault()}>
        <AdminAvatar user={adminUser} size={36} className="personal-center-button-avatar" />
      </Button>
    </Dropdown>
  )
}

function AdminAvatar({ user, size, className = '' }: { user: AdminUser; size: number; className?: string }) {
  const avatarClassName = `personal-center-avatar ${className}`.trim()
  if (user.avatar_url) {
    return <Avatar size={size} src={user.avatar_url} className={avatarClassName} />
  }
  return <Avatar size={size} className={`${avatarClassName} personal-center-avatar-fallback`}>{avatarInitial(user.username)}</Avatar>
}

function avatarInitial(name: string) {
  const [first = ''] = Array.from(name.trim())
  if (!first) {
    return 'A'
  }
  return /^[a-z]$/i.test(first) ? first.toUpperCase() : first
}

export function ClientInstallModal(props: {
  open: boolean
  loading: boolean
  saving: boolean
  form: ClientInstallCommandForm
  commandKind: ClientInstallCommandKind
  linuxCommand: string
  openWrtCommand: string
  windowsPowerShellCommand: string
  windowsCMDCommand: string
  onClose: () => void
  onSave: () => void
  onCopy: (command: string) => void
  onFormChange: (form: ClientInstallCommandForm) => void
  onCommandKindChange: (kind: ClientInstallCommandKind) => void
}) {
  const {
    open,
    loading,
    saving,
    form,
    commandKind,
    linuxCommand,
    openWrtCommand,
    windowsPowerShellCommand,
    windowsCMDCommand,
    onClose,
    onSave,
    onCopy,
    onFormChange,
    onCommandKindChange,
  } = props
  const activeCommand = clientInstallCommandByKind(commandKind, {
    linux: linuxCommand,
    openWrt: openWrtCommand,
    windowsPowerShell: windowsPowerShellCommand,
    windowsCMD: windowsCMDCommand,
  })
  const update = (patch: Partial<ClientInstallCommandForm>) => onFormChange({ ...form, ...patch })

  return (
    <Modal
      title="Client 一键安装命令"
      open={open}
      onCancel={onClose}
      width={820}
      style={{ top: 24 }}
      styles={{ body: { maxHeight: 'calc(100vh - 180px)', overflowY: 'auto', paddingRight: 4 } }}
      footer={[
        <Button key="cancel" onClick={onClose}>关闭</Button>,
        <Button key="save" loading={saving} onClick={onSave}>保存参数</Button>,
        <Button key="copy" type="primary" icon={<CopyOutlined />} disabled={loading} onClick={() => onCopy(activeCommand)}>复制当前分类命令</Button>,
      ]}
    >
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="复制命令到目标 VPS 上执行"
            description="这里会把 server 地址、注册 Token 和通用 client 参数写进 env，安装脚本会自动生成 client.json 并注册到当前 server。建议在目标 Linux 机器上使用 root 执行。"
          />
          {!form.registration_token.trim() ? (
            <Alert type="warning" showIcon message="缺少注册 Token" description="请先在 server 配置里填写 registration_token，否则 Client 安装后无法完成注册。" />
          ) : null}
          <Row gutter={[14, 14]}>
            <Col xs={24} md={12}>
              <Text type="secondary">Server 地址</Text>
              <Input value={form.server_url} placeholder="https://panel.example.com" onChange={(event) => update({ server_url: event.target.value })} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">Client 注册 Token</Text>
              <Input.Password value={form.registration_token} readOnly />
            </Col>
            <Col xs={24}>
              <Text type="secondary">安装脚本地址</Text>
              <Input value={form.install_script_url} onChange={(event) => update({ install_script_url: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">轮询间隔</Text>
              <Input value={form.poll_interval} placeholder="30s" onChange={(event) => update({ poll_interval: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">请求超时（秒）</Text>
              <InputNumber style={{ width: '100%' }} min={1} value={form.request_timeout_seconds} onChange={(value) => update({ request_timeout_seconds: Number(value || 15) })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">跳过 TLS 校验</Text>
              <div className="client-install-switch">
                <Switch checked={form.server_skip_tls_verify} onChange={(checked) => update({ server_skip_tls_verify: checked })} />
                <Text type="secondary">自签证书时开启</Text>
              </div>
            </Col>
          </Row>
          <Divider style={{ margin: '4px 0' }} />
          <Row gutter={[14, 14]} align="bottom">
            <Col xs={24} md={7}>
              <Text type="secondary">同时安装 Realm</Text>
              <div className="client-install-switch">
                <Switch
                  checked={form.realm_auto_install}
                  onChange={(checked) => update({
                    realm_auto_install: checked,
                    ...(checked ? { haproxy_auto_install: false } : {}),
                  })}
                />
                <Text type="secondary">Linux / OpenWrt</Text>
              </div>
            </Col>
            <Col xs={24} md={5}>
              <Text type="secondary">Realm 版本</Text>
              <Input disabled={!form.realm_auto_install} value={form.realm_version} placeholder="v2.9.4" onChange={(event) => update({ realm_version: event.target.value })} />
            </Col>
            <Col xs={24} md={12}>
              <Text type="secondary">Realm 下载镜像目录</Text>
              <Input disabled={!form.realm_auto_install} value={form.realm_download_base_url} placeholder="留空使用 GitHub 官方 Release" onChange={(event) => update({ realm_download_base_url: event.target.value })} />
            </Col>
          </Row>
          <Row gutter={[14, 14]} align="bottom">
            <Col xs={24} md={7}>
              <Text type="secondary">同时安装 HAProxy</Text>
              <div className="client-install-switch">
                <Switch
                  checked={form.haproxy_auto_install}
                  onChange={(checked) => update({
                    haproxy_auto_install: checked,
                    ...(checked ? { realm_auto_install: false } : {}),
                  })}
                />
                <Text type="secondary">Linux / OpenWrt</Text>
              </div>
            </Col>
          </Row>
          <Tabs
            activeKey={commandKind}
            onChange={(key) => onCommandKindChange(key as ClientInstallCommandKind)}
            items={[
              {
                key: 'linux',
                label: 'Debian / Ubuntu / CentOS',
                children: (
                  <ClientInstallCommandBox
                    title="Linux 安装命令"
                    description="在目标 Linux VPS 上使用 root 执行；自动识别 x86_64、ARM64、ARMv7，支持 systemd 和 OpenRC，并按上方设置安装 Realm / HAProxy。"
                    command={linuxCommand}
                    onCopy={() => onCopy(linuxCommand)}
                  />
                ),
              },
              {
                key: 'openwrt',
                label: 'iStoreOS / OpenWrt',
                children: (
                  <ClientInstallCommandBox
                    title="iStoreOS / OpenWrt 安装命令"
                    description="在路由系统 SSH 中使用 root 执行；自动识别 x86_64、ARM64、ARMv7，并按上方设置安装 Realm / HAProxy。"
                    command={openWrtCommand}
                    onCopy={() => onCopy(openWrtCommand)}
                  />
                ),
              },
              {
                key: 'windows-powershell',
                label: 'Windows PowerShell',
                children: (
                  <ClientInstallCommandBox
                    title="Windows PowerShell 安装命令"
                    description="在目标 Windows VPS 上以管理员身份打开 PowerShell 后执行，会安装为 Windows Service 并开机自启。"
                    command={windowsPowerShellCommand}
                    onCopy={() => onCopy(windowsPowerShellCommand)}
                  />
                ),
              },
              {
                key: 'windows-cmd',
                label: 'Windows CMD',
                children: (
                  <ClientInstallCommandBox
                    title="Windows CMD 安装命令"
                    description="在目标 Windows VPS 上以管理员身份打开 CMD 后执行。"
                    command={windowsCMDCommand}
                    onCopy={() => onCopy(windowsCMDCommand)}
                  />
                ),
              },
            ]}
          />
        </Space>
      </Spin>
    </Modal>
  )
}

export function AccountSettingsModal(props: {
  open: boolean
  saving: boolean
  form: AccountFormState
  onClose: () => void
  onSave: () => void
  onFormChange: (form: AccountFormState) => void
}) {
  const { open, saving, form, onClose, onSave, onFormChange } = props
  const [avatarError, setAvatarError] = useState('')
  const update = (patch: Partial<AccountFormState>) => onFormChange({ ...form, ...patch })
  const handleAvatarFile = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) {
      return
    }
    if (!file.type.startsWith('image/')) {
      setAvatarError('请选择图片文件')
      return
    }
    if (file.size > 768 * 1024) {
      setAvatarError('头像图片请控制在 768KB 以内')
      return
    }
    const reader = new FileReader()
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : ''
      if (!result) {
        setAvatarError('读取头像失败，请重试')
        return
      }
      setAvatarError('')
      update({ avatar_url: result })
    }
    reader.onerror = () => setAvatarError('读取头像失败，请重试')
    reader.readAsDataURL(file)
  }

  return (
    <Modal title="修改管理员账号" open={open} onCancel={onClose} onOk={onSave} confirmLoading={saving} okText="保存" cancelText="取消">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <div className="account-avatar-editor">
          <Avatar size={72} src={form.avatar_url || undefined} className="account-avatar-preview">
            {avatarInitial(form.new_username)}
          </Avatar>
          <div className="account-avatar-editor-actions">
            <Text strong>个人头像</Text>
            <Space wrap>
              <label className="avatar-upload-button">
                <input type="file" accept="image/*" onChange={handleAvatarFile} />
                <UploadOutlined />
                <span>更换头像</span>
              </label>
              {form.avatar_url ? <Button size="small" onClick={() => update({ avatar_url: '' })}>移除头像</Button> : null}
            </Space>
            <Text type={avatarError ? 'danger' : 'secondary'}>{avatarError || '支持 JPG / PNG / WebP，保存后会同步到个人中心。'}</Text>
          </div>
        </div>
        <Divider style={{ margin: '4px 0' }} />
        <div>
          <Text type="secondary">当前密码</Text>
          <Input.Password value={form.current_password} onChange={(event) => update({ current_password: event.target.value })} />
        </div>
        <div>
          <Text type="secondary">新用户名</Text>
          <Input value={form.new_username} onChange={(event) => update({ new_username: event.target.value })} />
        </div>
        <div>
          <Text type="secondary">新密码</Text>
          <Input.Password placeholder="留空表示不修改密码" value={form.new_password} onChange={(event) => update({ new_password: event.target.value })} />
        </div>
        <div>
          <Text type="secondary">确认新密码</Text>
          <Input.Password placeholder="留空表示不修改密码" value={form.confirm_password} onChange={(event) => update({ confirm_password: event.target.value })} />
        </div>
      </Space>
    </Modal>
  )
}

export function FrontendSettingsModal(props: {
  open: boolean
  loading: boolean
  saving: boolean
  form: FrontendSettingsForm
  onClose: () => void
  onSave: (form?: FrontendSettingsForm) => Promise<boolean>
  onFormChange: (form: FrontendSettingsForm) => void
}) {
  const { open, loading, saving, form, onClose, onSave, onFormChange } = props

  return (
    <Modal
      title="前端样式自定义"
      open={open}
      onCancel={() => { if (!saving) onClose() }}
      closable={!saving}
      maskClosable={!saving}
      keyboard={!saving}
      width={920}
      footer={[
        <Button key="cancel" disabled={saving} onClick={onClose}>关闭</Button>,
        <Button key="save" type="primary" disabled={loading} loading={saving} onClick={() => void onSave()}>保存并应用</Button>,
      ]}
    >
      <Spin spinning={loading}>
        <Text strong>管理员后台自定义代码（样式和脚本）</Text>
        <Input.TextArea
          value={form.custom_code}
          disabled={loading || saving}
          onChange={(event) => onFormChange({ ...form, custom_code: event.target.value })}
          autoSize={{ minRows: 16, maxRows: 28 }}
          placeholder={`<style>\n:root { --green: #2563eb; }\n</style>`}
        />
      </Spin>
    </Modal>
  )
}

export function FrontendSettingsPanel(props: {
  loading: boolean
  saving: boolean
  form: FrontendSettingsForm
  onSave: (form?: FrontendSettingsForm) => Promise<boolean>
}) {
  const { loading, saving, form, onSave } = props
  const [draftsOpen, setDraftsOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [announcementEditorOpen, setAnnouncementEditorOpen] = useState(false)
  const [editingAnnouncementIndex, setEditingAnnouncementIndex] = useState<number | null>(null)
  const [announcementDraft, setAnnouncementDraft] = useState<FrontendSettingsForm['announcements'][number] | null>(null)

  const deleteAnnouncement = (id: string) => onSave({
    ...form,
    announcements: form.announcements.filter((item) => item.id !== id),
  })

  const visibleAnnouncements = form.announcements.filter((item) => item.enabled)
  const draftAnnouncements = form.announcements.filter((item) => !item.enabled)

  const openNewAnnouncement = () => {
    setEditingAnnouncementIndex(null)
    setAnnouncementDraft(newCustomerAnnouncement())
    setAnnouncementEditorOpen(true)
  }

  const openAnnouncement = (index: number) => {
    setDraftsOpen(false)
    setEditingAnnouncementIndex(index)
    setAnnouncementDraft({ ...form.announcements[index] })
    setAnnouncementEditorOpen(true)
  }

  const closeAnnouncementEditor = () => {
    if (saving) return
    setAnnouncementEditorOpen(false)
    setEditingAnnouncementIndex(null)
    setAnnouncementDraft(null)
  }

  const saveAnnouncementDraft = async (enabled: boolean) => {
    if (saving || !announcementDraft) return
    if (enabled ? !announcementDraft.title.trim() : !announcementDraft.title.trim() && !(announcementDraft.content || '').trim()) return
    const next = { ...announcementDraft, enabled }
    const announcements = [...form.announcements]
    if (editingAnnouncementIndex === null) announcements.push(next)
    else announcements[editingAnnouncementIndex] = next
    if (await onSave({ ...form, announcements })) {
      setAnnouncementEditorOpen(false)
      setEditingAnnouncementIndex(null)
      setAnnouncementDraft(null)
    }
  }

  return (
    <div className="frontend-settings-page">
      <div className="admin-content-title">
        <div>
          <Typography.Title level={3}>客户公告</Typography.Title>
          <AdminHelpHint title="页面仅列出启用公告，新增与编辑通过弹窗完成；草稿不会展示给用户。" />
        </div>
        <Space>
          <Button disabled={loading || saving} onClick={() => setHistoryOpen(true)}>公告历史（{form.announcement_history.length}）</Button>
          <Button disabled={loading || saving} onClick={() => setDraftsOpen(true)}>草稿（{draftAnnouncements.length}）</Button>
          <Button type="primary" icon={<PlusOutlined />} disabled={loading || saving} onClick={openNewAnnouncement}>新增公告</Button>
        </Space>
      </div>
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {visibleAnnouncements.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无启用公告" /> : null}
          {visibleAnnouncements.map((announcement) => {
            const index = form.announcements.findIndex((item) => item.id === announcement.id)
            return (
            <Card
              key={announcement.id}
              size="small"
              className="admin-announcement-editor"
              title={announcement.title.trim() || `公告 ${index + 1}`}
              extra={(
                <Space size={4}>
                  <Tag color="success">启用</Tag>
                  <Button size="small" icon={<EditOutlined />} disabled={loading || saving} onClick={() => openAnnouncement(index)}>编辑</Button>
                  <Popconfirm title="确认删除这条公告？" onConfirm={() => deleteAnnouncement(announcement.id)}>
                    <Button type="text" danger disabled={loading || saving} icon={<DeleteOutlined />} title="删除公告" />
                  </Popconfirm>
                </Space>
              )}
            >
              <Text type="secondary">{announcement.content || '暂无公告内容'}</Text>
            </Card>
            )
          })}
          <Modal title={`公告草稿（${draftAnnouncements.length}）`} open={draftsOpen} onCancel={() => setDraftsOpen(false)} width={760} footer={<Button onClick={() => setDraftsOpen(false)}>关闭</Button>}>
              {draftAnnouncements.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无草稿" /> : null}
              <Space direction="vertical" style={{ width: '100%' }}>
                {draftAnnouncements.map((announcement) => {
                  const index = form.announcements.findIndex((item) => item.id === announcement.id)
                  return (
                    <Card key={announcement.id} size="small" title={announcement.title.trim() || '未命名草稿'} extra={<Space><Tag>草稿</Tag><Button size="small" disabled={loading || saving} icon={<EditOutlined />} onClick={() => openAnnouncement(index)}>继续编辑</Button><Popconfirm title="确认删除这条草稿？" onConfirm={() => deleteAnnouncement(announcement.id)}><Button size="small" disabled={loading || saving} danger icon={<DeleteOutlined />} title="删除草稿" /></Popconfirm></Space>}>
                      <Text type="secondary">{announcement.content || '暂无公告内容'}</Text>
                    </Card>
                  )
                })}
              </Space>
          </Modal>
          <Modal
            title={editingAnnouncementIndex === null ? '新增公告' : '编辑公告'}
            open={announcementEditorOpen}
            onCancel={closeAnnouncementEditor}
            closable={!saving}
            maskClosable={false}
            keyboard={!saving}
            width={760}
            footer={[
              <Button key="cancel" disabled={saving} onClick={closeAnnouncementEditor}>取消</Button>,
              <Button key="draft" loading={saving} onClick={() => void saveAnnouncementDraft(false)} disabled={loading || (!announcementDraft?.title.trim() && !(announcementDraft?.content || '').trim())}>保存草稿</Button>,
              <Button key="publish" loading={saving} type="primary" onClick={() => void saveAnnouncementDraft(true)} disabled={loading || !announcementDraft?.title.trim()}>发布公告</Button>,
            ]}
          >
            {announcementDraft ? (
              <fieldset disabled={saving} style={{ border: 0, padding: 0, margin: 0 }}><Row gutter={[12, 12]}>
                <Col xs={24} md={6}><Text strong>类型</Text><Select aria-label="公告类型" disabled={saving} value={announcementDraft.level || 'info'} style={{ width: '100%' }} options={[{ value: 'info', label: '通知' }, { value: 'success', label: '恢复' }, { value: 'warning', label: '提醒' }, { value: 'error', label: '紧急' }]} onChange={(level) => setAnnouncementDraft((current) => current ? { ...current, level } : current)} /></Col>
                <Col xs={24} md={18}><Text strong>标题</Text><Input aria-label="公告标题" value={announcementDraft.title} maxLength={120} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, title: event.target.value } : current)} /></Col>
                <Col span={24}><Text strong>内容</Text><Input.TextArea aria-label="公告内容" value={announcementDraft.content || ''} maxLength={1000} autoSize={{ minRows: 4, maxRows: 8 }} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, content: event.target.value } : current)} /></Col>
                <Col xs={24} md={8}><Text strong>链接按钮文字</Text><Input value={announcementDraft.link_label || ''} maxLength={60} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, link_label: event.target.value } : current)} /></Col>
                <Col xs={24} md={16}><Text strong>联系方式链接</Text><Input value={announcementDraft.link_url || ''} maxLength={500} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, link_url: event.target.value } : current)} /></Col>
                <Col xs={24} md={12}><Text strong>开始展示（可选）</Text><Input type="datetime-local" value={announcementDraft.starts_at || ''} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, starts_at: event.target.value } : current)} /></Col>
                <Col xs={24} md={12}><Text strong>结束展示（可选）</Text><Input type="datetime-local" value={announcementDraft.ends_at || ''} onChange={(event) => setAnnouncementDraft((current) => current ? { ...current, ends_at: event.target.value } : current)} /></Col>
              </Row></fieldset>
            ) : null}
          </Modal>
          <Modal title={`公告历史（${form.announcement_history.length}）`} open={historyOpen} onCancel={() => setHistoryOpen(false)} width={760} footer={<Button onClick={() => setHistoryOpen(false)}>关闭</Button>}>
              {form.announcement_history.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无公告历史" /> : null}
              {[...form.announcement_history].reverse().map((item) => (
                <div key={item.id} className="admin-announcement-history-item">
                  <div className="admin-announcement-history-meta">
                    <Tag color={announcementHistoryActionColor(item.action)}>
                      {announcementHistoryActionLabel(item.action)}
                    </Tag>
                    <Typography.Text strong>{item.title.trim() || '无标题公告'}</Typography.Text>
                    <Typography.Text type="secondary">
                      {formatAnnouncementHistoryTime(item.recorded_at)}
                    </Typography.Text>
                  </div>
                  {item.content?.trim() ? (
                    <p className="admin-announcement-history-content">{item.content}</p>
                  ) : null}
                </div>
              ))}
          </Modal>
        </Space>
      </Spin>
    </div>
  )
}

function newCustomerAnnouncement(): FrontendSettingsForm['announcements'][number] {
  return {
    id: typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : `announcement-${Date.now()}`,
    enabled: true,
    level: 'warning',
    title: '',
    content: '',
    link_label: '',
    link_url: '',
    starts_at: '',
    ends_at: '',
  }
}

function announcementHistoryActionLabel(action: string): string {
  switch (action) {
    case 'created':
      return '新增'
    case 'removed':
      return '删除'
    case 'updated':
    default:
      return '更新'
  }
}

function announcementHistoryActionColor(action: string): string {
  switch (action) {
    case 'created':
      return 'success'
    case 'removed':
      return 'error'
    case 'updated':
    default:
      return 'processing'
  }
}

function formatAnnouncementHistoryTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

export function ScheduledTasksPanel(props: {
  loading: boolean
  saving: boolean
  settings: ScheduledTaskSettings
  onSave: () => void
  onChange: (settings: ScheduledTaskSettings) => void
}) {
  const { loading, saving, settings, onSave, onChange } = props
  const update = (patch: Partial<ScheduledTaskSettings>) => onChange({ ...settings, ...patch })
  const updateAlertSweep = (patch: Partial<ScheduledTaskSettings['alert_sweep']>) => update({ alert_sweep: { ...settings.alert_sweep, ...patch } })
  const updateDailyReport = (patch: Partial<ScheduledTaskSettings['daily_traffic_report']>) => update({ daily_traffic_report: { ...settings.daily_traffic_report, ...patch } })

  return (
    <div className="frontend-settings-page">
      <div className="admin-content-title">
        <div>
          <Typography.Title level={3}>定时任务</Typography.Title>
          <AdminHelpHint title="管理 Server 后台任务的执行时间和频率；Client 到期同步任务已取消。" />
        </div>
        <Button type="primary" loading={saving} onClick={onSave}>保存任务配置</Button>
      </div>
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Card bordered={false} className="config-section-card">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <div className="admin-content-title compact">
                <div>
                  <Typography.Title level={4}>告警扫描</Typography.Title>
                  <AdminHelpHint title="用于即时发现 Client 离线；其他告警每天北京时间 09:00 统一扫描推送。" />
                </div>
                <Switch checked={settings.alert_sweep.enabled} onChange={(checked) => updateAlertSweep({ enabled: checked })} />
              </div>
              <Row gutter={[16, 16]}>
                <Col xs={24} md={8}>
                  <Text type="secondary">执行频率（分钟）</Text>
                  <InputNumber
                    style={{ width: '100%' }}
                    min={1}
                    max={1440}
                    value={settings.alert_sweep.interval_minutes || 5}
                    onChange={(value) => updateAlertSweep({ interval_minutes: Number(value || 5) })}
                  />
                </Col>
              </Row>
            </Space>
          </Card>
          <Card bordered={false} className="config-section-card">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <div className="admin-content-title compact">
                <div>
                  <Typography.Title level={4}>每日流量日报</Typography.Title>
                  <AdminHelpHint title="统计前一天流量，并在每天北京时间 09:00 推送到已启用的 Telegram Bot。" />
                </div>
                <Switch checked={settings.daily_traffic_report.enabled} onChange={(checked) => updateDailyReport({ enabled: checked })} />
              </div>
              <Row gutter={[16, 16]}>
                <Col xs={24} md={8}>
                  <Text type="secondary">执行时间</Text>
                  <Input
                    type="time"
                    value="09:00"
                    disabled
                  />
                </Col>
                <Col xs={24} md={8}>
                  <Text type="secondary">执行频率（天）</Text>
                  <InputNumber
                    style={{ width: '100%' }}
                    min={1}
                    max={365}
                    value={settings.daily_traffic_report.interval_days || 1}
                    onChange={(value) => updateDailyReport({ interval_days: Number(value || 1) })}
                  />
                </Col>
              </Row>
            </Space>
          </Card>
        </Space>
      </Spin>
    </div>
  )
}

export function TelegramBotSettingsModal(props: {
  open: boolean
  bots: TelegramBot[]
  loading: boolean
  saving: boolean
  editingID: number | null
  form: TelegramBotForm
  onClose: () => void
  onFormChange: (form: TelegramBotForm) => void
  onSave: () => void
  onRefresh: () => void
  onEditIDChange: (id: number | null) => void
  onDelete: (id: number) => void
  onTest: (id: number) => void
}) {
  const { open, bots, loading, saving, editingID, form, onClose, onFormChange, onSave, onRefresh, onEditIDChange, onDelete, onTest } = props

  return (
    <Modal title="Telegram 告警机器人" open={open} onCancel={onClose} footer={null} width={920}>
      <TelegramBotPanel
        bots={bots}
        loading={loading}
        saving={saving}
        editingID={editingID}
        form={form}
        onFormChange={onFormChange}
        onSave={onSave}
        onRefresh={onRefresh}
        onEdit={(bot) => {
          onEditIDChange(bot.id)
          onFormChange({ name: bot.name, bot_token: '', chat_id: bot.chat_id, enabled: bot.enabled })
        }}
        onCancelEdit={() => {
          onEditIDChange(null)
          onFormChange(defaultTelegramBotForm())
        }}
        onDelete={onDelete}
        onTest={onTest}
      />
    </Modal>
  )
}

export function XUIActionModal(props: {
  open: boolean
  saving: boolean
  actionKind: string
  addClientForm: XUIAddClientActionForm
  addClientInbounds?: XUINodeView[]
  outboundForm: XUIOutboundActionForm
  routingForm: XUIRoutingActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
  allowCreateOutbound?: boolean
  authorizedClientNodesOnly?: boolean
  onClose: () => void
  onSubmit: () => void
  onActionKindChange: (kind: string) => void
  onAddClientFormChange: (form: XUIAddClientActionForm) => void
  onOutboundFormChange: (form: XUIOutboundActionForm) => void
  onRoutingFormChange: (form: XUIRoutingActionForm) => void
}) {
  const {
    open,
    saving,
    actionKind,
    addClientForm,
    addClientInbounds,
    outboundForm,
    routingForm,
    agents,
    targetAgentID,
    currentOverview,
    sourceOverview,
    sourceLoading,
    allowCreateOutbound = true,
    authorizedClientNodesOnly = false,
    onClose,
    onSubmit,
    onActionKindChange,
    onAddClientFormChange,
    onOutboundFormChange,
    onRoutingFormChange,
  } = props

  return (
    <Modal title="下发 x-ui 操作" open={open} onCancel={onClose} onOk={onSubmit} confirmLoading={saving} okText="下发" cancelText="取消" width={920}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="执行方式"
          description="server 只保存任务；client 下一次轮询领取后，使用已托管的 x-ui 配置执行。客户端新增 / 启停 / 删除 / 到期调整不会重启 Xray；新增或修改出站、转发规则会重启 Xray 生效。"
        />
        <div>
          <Text type="secondary">操作类型</Text>
          <Select style={{ width: '100%' }} value={actionKind} options={XUI_ACTION_KINDS} onChange={onActionKindChange} />
        </div>
        {actionKind === 'add_client'
	          ? renderAddClientActionForm({
	              form: addClientForm,
	              inbounds: addClientInbounds?.length ? addClientInbounds : currentOverview?.nodes || [],
	              onChange: onAddClientFormChange,
	            })
          : null}
        {actionKind === 'add_outbound'
          ? renderOutboundActionForm({
              form: outboundForm,
              agents,
              targetAgentID,
              currentOverview,
              sourceOverview,
              sourceLoading,
              authorizedClientNodesOnly,
              onChange: onOutboundFormChange,
            })
          : null}
        {actionKind === 'add_routing_rule' || actionKind === 'upsert_routing_rule'
          ? renderRoutingActionForm({
              form: routingForm,
              outboundForm,
              agents,
              targetAgentID,
              currentOverview,
              sourceOverview,
              sourceLoading,
              inbounds: currentOverview?.nodes || [],
              clients: currentOverview?.clients || [],
              outbounds: currentOverview?.outbounds || [],
              balancers: currentOverview?.balancers || [],
              rules: currentOverview?.routing_rules || [],
              allowCreateOutbound,
              authorizedClientNodesOnly,
              onChange: onRoutingFormChange,
              onOutboundChange: onOutboundFormChange,
            })
          : null}
      </Space>
    </Modal>
  )
}

export function ImportURLModal(props: {
  client: XUIClientView | null
  onClose: () => void
  onCopy: (client: XUIClientView) => void
}) {
  const { client, onClose, onCopy } = props

  return (
    <Modal
      title="单节点导入 URL"
      open={Boolean(client)}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>关闭</Button>
          <Button type="primary" disabled={!client?.import_url} onClick={() => client && onCopy(client)}>复制 URL</Button>
        </Space>
      }
    >
      {client?.import_url ? (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text strong>{client.email || 'anonymous-client'}</Text>
            <div className="muted-line">{client.inbound_remark || client.inbound_tag || '-'}</div>
          </div>
          <div className="import-url-qr">
            <QRCode value={client.import_url} bordered={false} />
          </div>
          <Input.TextArea value={client.import_url} readOnly autoSize={{ minRows: 3, maxRows: 6 }} />
        </Space>
      ) : (
        <Empty description="当前客户端暂不支持生成单节点导入 URL" />
      )}
    </Modal>
  )
}


export function SystemUpdateModal(props: {
  open: boolean
  loading: boolean
  latestLoading: boolean
  latestInfo?: UpdateLatestInfo | null
  latestError?: string
  systemInfo?: SystemInfo | null
  onClose: () => void
  onRefreshLatest: () => void
  onUpdateServer: () => void
  onUpdateClients: () => void
}) {
  const { open, loading, latestLoading, latestInfo, latestError, systemInfo, onClose, onRefreshLatest, onUpdateServer, onUpdateClients } = props
  const serverUpdateAvailable = Boolean(latestInfo?.server_update_available)
  const clientUpdateCount = Number(latestInfo?.client_update_available_count || 0)
  const latestServerVersion = latestInfo?.latest_server_version || latestInfo?.latest_version || '-'
  const latestClientVersion = latestInfo?.latest_client_version || latestInfo?.latest_version || '-'

  return (
    <Modal title="在线升级" open={open} onCancel={onClose} footer={null} width={760}>
      <Spin spinning={latestLoading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="配置会保留"
          description="在线升级会复用现有 install.sh / install.ps1。server 会保留 server.json、数据库和 data；client 会保留 client.json。升级完成后服务会自动重启。"
        />
        {latestError ? <Alert type="error" showIcon message="获取最新版本失败" description={latestError} /> : null}
        {latestInfo ? (
          <Alert
            type={serverUpdateAvailable || clientUpdateCount > 0 ? 'success' : 'info'}
            showIcon
            message={`最新版本：Server v${latestServerVersion} · Client v${latestClientVersion}`}
            description={`当前 Server：v${latestInfo.current_server_version || systemInfo?.version || '-'}${systemInfo?.git_commit ? ` · 构建提交：${systemInfo.git_commit}` : ''}`}
          />
        ) : null}
        {latestInfo ? (
          <div className="update-status-grid">
            <div className="update-status-card">
              <Text type="secondary">Server</Text>
              <Tag color={serverUpdateAvailable ? 'blue' : 'default'}>{serverUpdateAvailable ? `可升级到 v${latestServerVersion}` : '已是最新'}</Tag>
            </div>
            <div className="update-status-card">
              <Text type="secondary">Client 可升级</Text>
              <Tag color={clientUpdateCount ? 'blue' : 'default'}>{clientUpdateCount ? `${clientUpdateCount} 台到 v${latestClientVersion}` : '0 台'}</Tag>
            </div>
            <div className="update-status-card">
              <Text type="secondary">已识别系统</Text>
              <Tag color="blue">{latestInfo.supported_client_count} 台</Tag>
            </div>
            <div className="update-status-card">
              <Text type="secondary">未知/不支持</Text>
              <Tag color={latestInfo.unknown_client_count || latestInfo.unsupported_client_count ? 'orange' : 'default'}>
                {latestInfo.unknown_client_count + latestInfo.unsupported_client_count} 台
              </Tag>
            </div>
          </div>
        ) : null}
        <Alert
          type="warning"
          showIcon
          message="先上传 GitHub Release"
          description="请先把最新的 server/client 包上传到 GitHub Release，否则在线升级会下载到旧包。"
        />
        <Space wrap>
          <Button onClick={onRefreshLatest} loading={latestLoading}>检查最新版本</Button>
          <Button type="primary" disabled={!serverUpdateAvailable} loading={loading} onClick={onUpdateServer}>升级当前 Server</Button>
          <Button disabled={clientUpdateCount <= 0} loading={loading} onClick={onUpdateClients}>下发升级到可升级 Client</Button>
        </Space>
        <AdminHelpHint title="Client 升级前会确认系统和架构，避免向不匹配的系统下发安装包。" />
        </Space>
      </Spin>
    </Modal>
  )
}
