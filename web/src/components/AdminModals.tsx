import { useState, type ChangeEvent } from 'react'
import { Alert, Avatar, Button, Col, Divider, Dropdown, Empty, Input, InputNumber, Modal, QRCode, Row, Select, Space, Spin, Switch, Tabs, Tag, Typography } from 'antd'
import type { MenuProps } from 'antd'
import {
  BellOutlined,
  CloudDownloadOutlined,
  CopyOutlined,
  EditOutlined,
  LogoutOutlined,
  SettingOutlined,
  TeamOutlined,
  UploadOutlined,
} from '@ant-design/icons'

import type { AdminUser, AgentListItem, SystemInfo, TelegramBot, UpdateLatestInfo, XUIClientView, XUIOverview } from '../types'
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
          <Divider orientation="left">VPS 初始化：自动安装 x-ui</Divider>
          <Alert
            type="info"
            showIcon
            message="新 Client 首次注册后会继承这里的 x-ui 初始化配置"
            description="开启后，Linux VPS 上的 client 会自动安装 3x-ui，并统一配置后台账号、密码、端口和 web path。已单独配置过 x-ui 的 client 不会被覆盖。"
          />
          <Row gutter={[14, 14]}>
            <Col xs={24} md={8}>
              <Text type="secondary">自动安装 x-ui</Text>
              <div className="client-install-switch">
                <Switch checked={form.xui_auto_install} onChange={(checked) => update({ xui_auto_install: checked })} />
                <Text type="secondary">仅 Linux VPS 生效</Text>
              </div>
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">x-ui 账号</Text>
              <Input value={form.xui_username} placeholder="admin" disabled={!form.xui_auto_install} onChange={(event) => update({ xui_username: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">x-ui 密码</Text>
              <Input.Password value={form.xui_password} placeholder="建议使用强密码" disabled={!form.xui_auto_install} onChange={(event) => update({ xui_password: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">面板端口</Text>
              <InputNumber style={{ width: '100%' }} min={1} max={65535} value={form.xui_panel_port} disabled={!form.xui_auto_install} onChange={(value) => update({ xui_panel_port: Number(value || 2053) })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">Web Path</Text>
              <Input value={form.xui_web_path} placeholder="/xui/" disabled={!form.xui_auto_install} onChange={(event) => update({ xui_web_path: event.target.value })} />
            </Col>
            <Col xs={24} md={8}>
              <Text type="secondary">3x-ui 安装脚本</Text>
              <Input value={form.xui_install_script_url} disabled={!form.xui_auto_install} onChange={(event) => update({ xui_install_script_url: event.target.value })} />
            </Col>
          </Row>
          <Tabs
            activeKey={commandKind}
            onChange={(key) => onCommandKindChange(key as ClientInstallCommandKind)}
            items={[
              {
                key: 'linux',
                label: 'Linux / Alpine',
                children: (
                  <ClientInstallCommandBox
                    title="Linux / Alpine 安装命令"
                    description="在目标 Linux VPS 上使用 root 执行；支持 systemd 和 OpenRC。"
                    command={linuxCommand}
                    onCopy={() => onCopy(linuxCommand)}
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
  onSave: () => void
  onFormChange: (form: FrontendSettingsForm) => void
}) {
  const { open, loading, saving, form, onClose, onSave, onFormChange } = props

  return (
    <Modal
      title="管理员后台样式自定义"
      open={open}
      onCancel={onClose}
      width={920}
      footer={[
        <Button key="cancel" onClick={onClose}>关闭</Button>,
        <Button key="save" type="primary" loading={saving} onClick={onSave}>保存并应用</Button>,
      ]}
    >
      <FrontendSettingsPanel
        loading={loading}
        saving={saving}
        form={form}
        onSave={onSave}
        onFormChange={onFormChange}
      />
    </Modal>
  )
}

export function FrontendSettingsPanel(props: {
  loading: boolean
  saving: boolean
  form: FrontendSettingsForm
  onSave: () => void
  onFormChange: (form: FrontendSettingsForm) => void
}) {
  const { loading, saving, form, onSave, onFormChange } = props

  return (
    <div className="frontend-settings-page">
      <div className="admin-content-title">
        <div>
          <Typography.Title level={3}>系统设置</Typography.Title>
          <Text type="secondary">配置管理员后台自定义样式；用户账号样式仍在用户看板里单独配置。</Text>
        </div>
        <Button type="primary" loading={saving} onClick={onSave}>保存并应用</Button>
      </div>
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="只影响管理员后台"
            description="用户账号不会继承这里的样式；每个用户账号请在用户页“页面样式”里单独保存。背景图可以用 window.CustomBackgroundImage = '图片地址'。"
          />
          <div>
            <Text strong>自定义代码（样式和脚本）</Text>
            <Input.TextArea
              value={form.custom_code}
              onChange={(event) => onFormChange({ custom_code: event.target.value })}
              autoSize={{ minRows: 16, maxRows: 28 }}
              placeholder={`<style>\n:root { --green: #2563eb; }\n</style>\n<script>\nwindow.CustomBackgroundImage = 'https://example.com/bg.jpg'\n</script>`}
            />
          </div>
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
  outboundForm: XUIOutboundActionForm
  routingForm: XUIRoutingActionForm
  agents: AgentListItem[]
  targetAgentID: string
  currentOverview: XUIOverview | null
  sourceOverview: XUIOverview | null
  sourceLoading: boolean
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
    outboundForm,
    routingForm,
    agents,
    targetAgentID,
    currentOverview,
    sourceOverview,
    sourceLoading,
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
          description="server 只保存任务；client 下一次轮询领取后，使用已托管的 x-ui 账号密码调用 3x-ui API 执行。支持在节点下新增客户端、导入内部 Client 出站，以及配置转发规则。"
        />
        <div>
          <Text type="secondary">操作类型</Text>
          <Select style={{ width: '100%' }} value={actionKind} options={XUI_ACTION_KINDS} onChange={onActionKindChange} />
        </div>
        {actionKind === 'add_client'
          ? renderAddClientActionForm({
              form: addClientForm,
              inbounds: currentOverview?.nodes || [],
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
  onUpdate3XUI: () => void
  force3XUIUpdate: boolean
  onForce3XUIUpdateChange: (value: boolean) => void
}) {
  const { open, loading, latestLoading, latestInfo, latestError, systemInfo, onClose, onRefreshLatest, onUpdateServer, onUpdateClients, onUpdate3XUI, force3XUIUpdate, onForce3XUIUpdateChange } = props
  const serverUpdateAvailable = Boolean(latestInfo?.server_update_available)
  const clientUpdateCount = Number(latestInfo?.client_update_available_count || 0)
  const xuiUpdateCount = Number(latestInfo?.xui_update_available_count || 0)
  const xuiUnknownCount = Number(latestInfo?.unknown_xui_count || 0)
  const canSubmit3XUIUpdate = force3XUIUpdate || xuiUpdateCount > 0 || xuiUnknownCount > 0
  const latestServerVersion = latestInfo?.latest_server_version || latestInfo?.latest_version || '-'
  const latestClientVersion = latestInfo?.latest_client_version || latestInfo?.latest_version || '-'
  const latest3XUIVersion = latestInfo?.latest_3xui_version || '-'

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
        {latestInfo?.latest_3xui_error ? <Alert type="warning" showIcon message="获取 3x-ui 最新版本失败" description={latestInfo.latest_3xui_error} /> : null}
        {latestInfo ? (
          <Alert
            type={serverUpdateAvailable || clientUpdateCount > 0 || xuiUpdateCount > 0 ? 'success' : 'info'}
            showIcon
            message={`最新版本：Server v${latestServerVersion} · Client v${latestClientVersion} · 3x-ui v${latest3XUIVersion}`}
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
              <Text type="secondary">3x-ui 可升级</Text>
              <Tag color={xuiUpdateCount ? 'blue' : 'default'}>{xuiUpdateCount ? `${xuiUpdateCount} 台到 v${latest3XUIVersion}` : '0 台'}</Tag>
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
            <div className="update-status-card">
              <Text type="secondary">3x-ui 未识别</Text>
              <Tag color={latestInfo.unknown_xui_count || latestInfo.unsupported_xui_count ? 'orange' : 'default'}>
                {latestInfo.unknown_xui_count + latestInfo.unsupported_xui_count} 台
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
          <Button disabled={!canSubmit3XUIUpdate} loading={loading} onClick={onUpdate3XUI}>{force3XUIUpdate ? '强制批量升级 3x-ui' : '批量升级 3x-ui'}</Button>
          <Space size={8}>
            <Switch checked={force3XUIUpdate} onChange={onForce3XUIUpdateChange} />
            <Text type="secondary">强制升级 3x-ui</Text>
          </Space>
        </Space>
        <Text type="secondary">Client 升级前会确认系统和架构；3x-ui 点击升级时会重新检测 GitHub 最新版本，任务执行时 Client 会再次读取本机 3x-ui 版本并比较；开启强制升级后不比较版本，直接执行官方 update.sh。</Text>
        </Space>
      </Spin>
    </Modal>
  )
}
