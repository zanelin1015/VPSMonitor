import { useState } from 'react'
import { Alert, Button, Input, InputNumber, Modal, Select, Space, Tag, Typography } from 'antd'
import type { Dispatch, SetStateAction } from 'react'
import type { XUIAction } from '../types'
import type { AgentDetailPanelProps } from './AgentDetailPanel.types'
import { actionStatusLabel } from '../lib/appHelpers'
import { defaultTerminalShell, RemoteTTYTerminal } from './AgentDetailTerminal'

const { Text } = Typography

type AgentTerminalOptions = Pick<AgentDetailPanelProps, 'selectedAgent' | 'selectedAgentId' | 'remoteCommandLoading' | 'onExecuteRemoteCommand'> & {
  commandOutputAction: XUIAction | null
  setCommandOutputAction: Dispatch<SetStateAction<XUIAction | null>>
}

export function useAgentTerminalTools({ selectedAgent, selectedAgentId, remoteCommandLoading, onExecuteRemoteCommand, commandOutputAction, setCommandOutputAction }: AgentTerminalOptions) {
  const [remoteCommandOpen, setRemoteCommandOpen] = useState(false)
  const [remoteCommand, setRemoteCommand] = useState('')
  const [remoteShell, setRemoteShell] = useState('bash')
  const [remoteTimeout, setRemoteTimeout] = useState(120)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [terminalShell, setTerminalShell] = useState(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
  const [terminalFontSize, setTerminalFontSize] = useState(13)
  const [terminalExpanded, setTerminalExpanded] = useState(false)

  const commandResult = commandOutputAction?.result || {}
  const commandPayload = commandOutputAction?.payload || {}
  const commandText = String(commandResult.command || commandPayload.command || '')
  const stdoutText = String(commandResult.stdout || '')
  const stderrText = String(commandResult.stderr || commandOutputAction?.error || '')
  function submitRemoteCommand() {
    const command = remoteCommand.trim()
    if (!command) {
      return
    }
    onExecuteRemoteCommand(command, remoteShell, remoteTimeout)
    setRemoteCommandOpen(false)
  }

  function openRealtimeTerminal() {
    setTerminalShell(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
    setTerminalOpen(true)
  }

  function openRemoteCommand() {
    setRemoteShell(defaultTerminalShell(selectedAgent.client_os, selectedAgent.system_version))
    setRemoteCommandOpen(true)
  }

  function zoomTerminal(delta: number) {
    setTerminalFontSize((value) => Math.min(22, Math.max(10, value + delta)))
  }

  function resetTerminalZoom() {
    setTerminalFontSize(13)
    setTerminalExpanded(false)
  }

  const terminalModals = <>
      <Modal
        title="实时 TTY"
        open={terminalOpen}
        footer={null}
        onCancel={() => setTerminalOpen(false)}
        width={terminalExpanded ? 'min(1500px, 98vw)' : 'min(1100px, 96vw)'}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="这是直接连到 Client 服务权限的实时终端"
            description="Linux 使用 PTY，会以 vpsmonitor-client 服务用户打开交互式 shell；Windows 使用管理员服务权限下的 PowerShell/CMD 管道。请只在可信设备上操作。"
          />
          <Space wrap>
            <Text type="secondary">Shell</Text>
            <Select
              style={{ width: 190 }}
              value={terminalShell}
              options={[
                { value: 'bash', label: 'Linux bash' },
                { value: 'sh', label: 'Linux sh' },
                { value: 'powershell', label: 'Windows PowerShell' },
                { value: 'cmd', label: 'Windows CMD' },
                { value: 'pwsh', label: 'PowerShell Core' },
              ]}
              onChange={setTerminalShell}
            />
            <Text type="secondary">切换 Shell 会重新建立终端连接</Text>
            <Space.Compact>
              <Button onClick={() => zoomTerminal(-1)} disabled={terminalFontSize <= 10}>
                缩小
              </Button>
              <Button disabled>{terminalFontSize}px</Button>
              <Button onClick={() => zoomTerminal(1)} disabled={terminalFontSize >= 22}>
                放大
              </Button>
            </Space.Compact>
            <Button onClick={resetTerminalZoom} disabled={terminalFontSize === 13 && !terminalExpanded}>
              重置缩放
            </Button>
            <Button onClick={() => setTerminalExpanded((value) => !value)}>
              {terminalExpanded ? '普通窗口' : '放大窗口'}
            </Button>
          </Space>
          <RemoteTTYTerminal agentID={selectedAgentId} shell={terminalShell} active={terminalOpen} fontSize={terminalFontSize} expanded={terminalExpanded} />
        </Space>
      </Modal>
      <Modal
        title="后台命令行"
        open={remoteCommandOpen}
        okText="下发执行"
        cancelText="取消"
        confirmLoading={remoteCommandLoading}
        okButtonProps={{ danger: true, disabled: !remoteCommand.trim() }}
        onOk={submitRemoteCommand}
        onCancel={() => setRemoteCommandOpen(false)}
        destroyOnClose
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Alert
            type="warning"
            showIcon
            message="命令会直接通过 Client 执行"
            description="Linux 通常以 vpsmonitor-client 服务用户执行（安装为 root 时就是 root 权限）；Windows 取决于 Client 服务是否以管理员权限运行。命令和输出会记录在 x-ui 操作记录中。"
          />
          <Space wrap>
            <div>
              <Text type="secondary">Shell</Text>
              <Select
                style={{ width: 170, display: 'block', marginTop: 6 }}
                value={remoteShell}
                options={[
                  { value: 'bash', label: 'Linux bash' },
                  { value: 'sh', label: 'Linux sh' },
                  { value: 'powershell', label: 'Windows PowerShell' },
                  { value: 'cmd', label: 'Windows CMD' },
                  { value: 'pwsh', label: 'PowerShell Core' },
                ]}
                onChange={setRemoteShell}
              />
            </div>
            <div>
              <Text type="secondary">超时时间（秒）</Text>
              <InputNumber
                min={1}
                max={600}
                style={{ width: 140, display: 'block', marginTop: 6 }}
                value={remoteTimeout}
                onChange={(value) => setRemoteTimeout(Number(value || 120))}
              />
            </div>
          </Space>
          <Input.TextArea
            autoSize={{ minRows: 6, maxRows: 12 }}
            placeholder="例如：systemctl status x-ui --no-pager"
            value={remoteCommand}
            onChange={(event) => setRemoteCommand(event.target.value)}
          />
        </Space>
      </Modal>
      <Modal
        title={commandOutputAction?.kind === 'update_3xui' ? '3x-ui 升级输出' : '远程命令输出'}
        open={Boolean(commandOutputAction)}
        footer={<Button onClick={() => setCommandOutputAction(null)}>关闭</Button>}
        onCancel={() => setCommandOutputAction(null)}
        width={860}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text type="secondary">命令</Text>
            <pre className="command-output-block">{commandText || '-'}</pre>
          </div>
          <Space wrap>
            <Tag color={commandOutputAction?.status === 'succeeded' ? 'success' : 'error'}>{actionStatusLabel(commandOutputAction?.status || '')}</Tag>
            <Tag>Shell {String(commandResult.shell || commandPayload.shell || '-')}</Tag>
            <Tag>退出码 {String(commandResult.exit_code ?? '-')}</Tag>
            <Tag>执行用户 {String(commandResult.run_as || commandResult.uid || '-')}</Tag>
            <Tag>耗时 {String(commandResult.duration || '-')}</Tag>
          </Space>
          <div>
            <Text type="secondary">STDOUT</Text>
            <pre className="command-output-block">{stdoutText || '-'}</pre>
          </div>
          <div>
            <Text type="secondary">STDERR / 错误</Text>
            <pre className="command-output-block">{stderrText || '-'}</pre>
          </div>
        </Space>
      </Modal>
  </>

  return { openRealtimeTerminal, openRemoteCommand, terminalModals }
}
