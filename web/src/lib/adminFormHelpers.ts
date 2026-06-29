import type { ClientInstallInfo, FrontendSettings } from '../types'

export type ClientInstallCommandKind = 'linux' | 'windows-powershell' | 'windows-cmd'

export interface TelegramBotForm {
  name: string
  bot_token: string
  chat_id: string
  enabled: boolean
}

export interface ClientInstallCommandForm {
  server_url: string
  registration_token: string
  install_script_url: string
  poll_interval: string
  request_timeout_seconds: number
  server_skip_tls_verify: boolean
  xui_auto_install: boolean
  xui_username: string
  xui_password: string
  xui_panel_port: number
  xui_web_path: string
  xui_install_script_url: string
}

export interface FrontendSettingsForm {
  custom_code: string
}

export function defaultTelegramBotForm(): TelegramBotForm {
  return {
    name: '',
    bot_token: '',
    chat_id: '',
    enabled: true,
  }
}

export function defaultClientInstallCommandForm(): ClientInstallCommandForm {
  return {
    server_url: typeof window !== 'undefined' ? window.location.origin : 'http://SERVER_IP:8090',
    registration_token: '',
    install_script_url: 'https://raw.githubusercontent.com/zanelin1015/VPSMonitor/main/install.sh',
    poll_interval: '30s',
    request_timeout_seconds: 15,
    server_skip_tls_verify: false,
    xui_auto_install: false,
    xui_username: 'admin',
    xui_password: '',
    xui_panel_port: 2053,
    xui_web_path: '/xui/',
    xui_install_script_url: 'https://raw.githubusercontent.com/MHSanaei/3x-ui/master/install.sh',
  }
}

export function normalizeClientInstallCommandForm(info: ClientInstallInfo): ClientInstallCommandForm {
  return {
    server_url: info.server_url || defaultClientInstallCommandForm().server_url,
    registration_token: info.registration_token || '',
    install_script_url: info.install_script_url || defaultClientInstallCommandForm().install_script_url,
    poll_interval: info.poll_interval || '30s',
    request_timeout_seconds: Number(info.request_timeout_seconds || 15),
    server_skip_tls_verify: Boolean(info.server_skip_tls_verify),
    xui_auto_install: Boolean(info.xui_auto_install),
    xui_username: info.xui_username || 'admin',
    xui_password: info.xui_password || '',
    xui_panel_port: Number(info.xui_panel_port || 2053),
    xui_web_path: info.xui_web_path || '/xui/',
    xui_install_script_url: info.xui_install_script_url || defaultClientInstallCommandForm().xui_install_script_url,
  }
}

export function defaultFrontendSettingsForm(): FrontendSettingsForm {
  return { custom_code: '' }
}

export function normalizeFrontendSettingsForm(settings: FrontendSettings): FrontendSettingsForm {
  return { custom_code: settings.custom_code || '' }
}

export function clientInstallCommandByKind(
  kind: ClientInstallCommandKind,
  commands: { linux: string; windowsPowerShell: string; windowsCMD: string },
): string {
  switch (kind) {
    case 'windows-powershell':
      return commands.windowsPowerShell
    case 'windows-cmd':
      return commands.windowsCMD
    case 'linux':
    default:
      return commands.linux
  }
}

export function buildClientInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = form.install_script_url.trim() || defaultClientInstallCommandForm().install_script_url
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `${key}=${shellQuote(value)}`).join(' ')
  return `curl -L ${shellQuote(scriptURL)} -o vpsmonitor-install.sh && chmod +x vpsmonitor-install.sh && env ${envText} ./vpsmonitor-install.sh client`
}

export function buildWindowsPowerShellInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = windowsInstallScriptURL(form.install_script_url)
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `$env:${key}=${powerShellQuote(value)}`).join('; ')
  return `${envText}; $script=Join-Path $env:TEMP 'vpsmonitor-install.ps1'; Remove-Item -Force $script -ErrorAction SilentlyContinue; iwr -UseBasicParsing -Headers @{'Cache-Control'='no-cache'} ${powerShellQuote(scriptURL)} -OutFile $script; Select-String -Path $script -Pattern 'InstallerVersion' | Write-Host; powershell -NoProfile -ExecutionPolicy Bypass -File $script client`
}

export function buildWindowsCMDInstallCommand(form: ClientInstallCommandForm): string {
  return `powershell -NoProfile -ExecutionPolicy Bypass -Command ${powerShellQuote(buildWindowsPowerShellInstallCommand(form))}`
}

export function windowsInstallScriptURL(scriptURL: string): string {
  const value = (scriptURL || defaultClientInstallCommandForm().install_script_url).trim()
  const psURL = value.endsWith('.sh') ? `${value.slice(0, -3)}.ps1` : value
  if (psURL.includes('raw.githubusercontent.com') && !psURL.includes('?')) {
    return `${psURL}?v=2026050902`
  }
  return psURL
}

export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

export function powerShellQuote(value: string): string {
  return `'${value.replace(/'/g, `''`)}'`
}
