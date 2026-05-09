import { Alert, Button, Col, Empty, Input, InputNumber, Modal, QRCode, Row, Select, Space, Spin, Switch, Tabs, Tag, Typography } from 'antd'
import {
  BellOutlined,
  CloudDownloadOutlined,
  CopyOutlined,
  EditOutlined,
  LogoutOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
} from '@ant-design/icons'

import type { AdminUser, AgentListItem, SystemInfo, TelegramBot, XUIClientView, XUIOverview } from '../types'
import type {
  ClientInstallCommandForm,
  ClientInstallCommandKind,
  FrontendSettingsForm,
  TelegramBotForm,
  XUIOutboundActionForm,
  XUIRoutingActionForm,
} from '../lib/appHelpers'
import { XUI_ACTION_KINDS, clientInstallCommandByKind, defaultTelegramBotForm } from '../lib/appHelpers'
import { ClientInstallCommandBox } from './ClientInstallCommandBox'
import { TelegramBotPanel } from './TelegramBotPanel'
import { renderOutboundActionForm, renderRoutingActionForm } from './XUIActionForms'

const { Text, Title } = Typography

export interface AccountFormState {
  current_password: string
  new_username: string
  new_password: string
  confirm_password: string
}

export function PersonalCenterModal(props: {
  open: boolean
  adminUser: AdminUser
  systemInfo?: SystemInfo | null
  onClose: () => void
  onOpenAccount: () => void
  onOpenClientInstall: () => void
  onOpenTelegram: () => void
  onOpenFrontendSettings: () => void
  onOpenUpdates: () => void
  onLogout: () => void
}) {
  const { open, adminUser, systemInfo, onClose, onOpenAccount, onOpenClientInstall, onOpenTelegram, onOpenFrontendSettings, onOpenUpdates, onLogout } = props

  return (
    <Modal title="个人中心" open={open} onCancel={onClose} footer={null} width={520}>
      <div className="personal-center-panel">
        <div className="personal-center-profile">
          <div className="personal-center-avatar">
            <SafetyCertificateOutlined />
          </div>
          <div>
            <Text type="secondary">当前管理员</Text>
            <Title level={3}>{adminUser.username}</Title>
            <Space wrap size={6}>
              <Tag color="success">已登录</Tag>
              {systemInfo?.version ? <Tag color="blue">Server v{systemInfo.version}</Tag> : null}
            </Space>
          </div>
        </div>
        <div className="personal-center-actions">
          <Button icon={<EditOutlined />} onClick={onOpenAccount}>修改账号密码</Button>
          <Button icon={<CloudDownloadOutlined />} onClick={onOpenClientInstall}>Client 安装命令</Button>
          <Button icon={<BellOutlined />} onClick={onOpenTelegram}>TG 告警机器人</Button>
          <Button icon={<SettingOutlined />} onClick={onOpenFrontendSettings}>前端样式自定义</Button>
          <Button icon={<SettingOutlined />} onClick={onOpenUpdates}>在线更新</Button>
          <Button danger icon={<LogoutOutlined />} onClick={onLogout}>退出登录</Button>
        </div>
      </div>
    </Modal>
  )
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
  const update = (patch: Partial<AccountFormState>) => onFormChange({ ...form, ...patch })

  return (
    <Modal title="修改管理员账号" open={open} onCancel={onClose} onOk={onSave} confirmLoading={saving} okText="保存" cancelText="取消">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
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
      title="前端样式自定义"
      open={open}
      onCancel={onClose}
      width={920}
      footer={[
        <Button key="cancel" onClick={onClose}>关闭</Button>,
        <Button key="save" type="primary" loading={saving} onClick={onSave}>保存并应用</Button>,
      ]}
    >
      <Spin spinning={loading}>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="支持自定义 CSS / HTML / script"
            description="这里的代码会在所有访问者浏览器中执行，请只填写你信任的样式和脚本。背景图可以用 window.CustomBackgroundImage = '图片地址'。"
          />
          <div>
            <Text strong>自定义代码（样式和脚本）</Text>
            <Input.TextArea
              value={form.custom_code}
              onChange={(event) => onFormChange({ custom_code: event.target.value })}
              autoSize={{ minRows: 12, maxRows: 22 }}
              placeholder={`<style>\n:root { --green: #22c55e; }\n</style>\n<script>\nwindow.CustomBackgroundImage = 'https://example.com/bg.jpg'\n</script>`}
            />
          </div>
        </Space>
      </Spin>
    </Modal>
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
  onOutboundFormChange: (form: XUIOutboundActionForm) => void
  onRoutingFormChange: (form: XUIRoutingActionForm) => void
}) {
  const {
    open,
    saving,
    actionKind,
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
          description="server 只保存任务；client 下一次轮询领取后，使用已托管的 x-ui 账号密码调用 3x-ui API 执行。这里仅允许把内部 Client 节点导入为出站，再配置转发规则。"
        />
        <div>
          <Text type="secondary">操作类型</Text>
          <Select style={{ width: '100%' }} value={actionKind} options={XUI_ACTION_KINDS} onChange={onActionKindChange} />
        </div>
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
        {actionKind === 'add_routing_rule'
          ? renderRoutingActionForm({
              form: routingForm,
              inbounds: currentOverview?.nodes || [],
              clients: currentOverview?.clients || [],
              outbounds: currentOverview?.outbounds || [],
              onChange: onRoutingFormChange,
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
  systemInfo?: SystemInfo | null
  onClose: () => void
  onUpdateServer: () => void
  onUpdateClients: () => void
}) {
  const { open, loading, systemInfo, onClose, onUpdateServer, onUpdateClients } = props

  return (
    <Modal title="在线更新" open={open} onCancel={onClose} footer={null} width={680}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="配置会保留"
          description="在线更新会复用现有 install.sh / install.ps1。server 会保留 server.json、数据库和 data；client 会保留 client.json。更新完成后服务会自动重启。"
        />
        {systemInfo?.version ? (
          <Alert
            type="success"
            showIcon
            message={`当前 Server 版本：v${systemInfo.version}`}
            description={systemInfo.git_commit ? `构建提交：${systemInfo.git_commit}` : '版本来自当前运行中的 server 二进制。'}
          />
        ) : null}
        <Alert
          type="warning"
          showIcon
          message="先上传 GitHub Release"
          description="请先把最新的 server/client 包上传到 GitHub Release，否则在线更新会下载到旧包。"
        />
        <Space wrap>
          <Button type="primary" loading={loading} onClick={onUpdateServer}>更新当前 Server</Button>
          <Button loading={loading} onClick={onUpdateClients}>下发更新到所有 Client</Button>
        </Space>
        <Text type="secondary">Client 更新任务会在每个 client 下一次轮询时领取；数量多时无需逐台 SSH。</Text>
      </Space>
    </Modal>
  )
}
