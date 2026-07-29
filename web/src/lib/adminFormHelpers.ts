import type { ClientInstallInfo, FrontendSettings } from '../types'

export type ClientInstallCommandKind = 'linux' | 'openwrt' | 'windows-powershell' | 'windows-cmd'

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
  realm_auto_install: boolean
  realm_version: string
  realm_download_base_url: string
  haproxy_auto_install: boolean
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
    realm_auto_install: false,
    realm_version: 'v2.9.4',
    realm_download_base_url: '',
    haproxy_auto_install: false,
    xui_auto_install: false,
    xui_username: 'admin',
    xui_password: '',
    xui_panel_port: 2053,
    xui_web_path: '/xui/',
    xui_install_script_url: 'https://raw.githubusercontent.com/MHSanaei/3x-ui/master/install.sh',
  }
}

export function normalizeClientInstallCommandForm(info: ClientInstallInfo): ClientInstallCommandForm {
  const haproxyAutoInstall = Boolean(info.haproxy_auto_install)
  return {
    server_url: info.server_url || defaultClientInstallCommandForm().server_url,
    registration_token: info.registration_token || '',
    install_script_url: info.install_script_url || defaultClientInstallCommandForm().install_script_url,
    poll_interval: info.poll_interval || '30s',
    request_timeout_seconds: Number(info.request_timeout_seconds || 15),
    server_skip_tls_verify: Boolean(info.server_skip_tls_verify),
    realm_auto_install: Boolean(info.realm_auto_install) && !haproxyAutoInstall,
    realm_version: info.realm_version || 'v2.9.4',
    realm_download_base_url: info.realm_download_base_url || '',
    haproxy_auto_install: haproxyAutoInstall,
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
  commands: { linux: string; openWrt: string; windowsPowerShell: string; windowsCMD: string },
): string {
  switch (kind) {
    case 'openwrt':
      return commands.openWrt
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
  const haproxyAutoInstall = Boolean(form.haproxy_auto_install)
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_REALM_AUTO_INSTALL', String(Boolean(form.realm_auto_install) && !haproxyAutoInstall)],
    ['VPSMONITOR_REALM_VERSION', form.realm_version.trim() || 'v2.9.4'],
    ['VPSMONITOR_REALM_DOWNLOAD_BASE_URL', form.realm_download_base_url.trim()],
    ['VPSMONITOR_HAPROXY_AUTO_INSTALL', String(haproxyAutoInstall)],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `${key}=${shellQuote(value)}`).join(' ')
  const quotedURL = shellQuote(scriptURL)
  return `tmp=./vpsmonitor-install.sh; if command -v curl >/dev/null 2>&1; then curl -fL ${quotedURL} -o "$tmp"; else wget -O "$tmp" ${quotedURL}; fi && chmod +x "$tmp" && env ${envText} "$tmp" client`
}

export function buildOpenWrtInstallCommand(form: ClientInstallCommandForm): string {
  const scriptURL = openWrtInstallScriptURL(form.install_script_url)
  const haproxyAutoInstall = Boolean(form.haproxy_auto_install)
  const envValues: Array<[string, string]> = [
    ['VPSMONITOR_SERVER_URL', form.server_url.trim()],
    ['VPSMONITOR_REGISTRATION_TOKEN', form.registration_token.trim()],
    ['VPSMONITOR_SERVER_SKIP_TLS_VERIFY', String(Boolean(form.server_skip_tls_verify))],
    ['VPSMONITOR_POLL_INTERVAL', form.poll_interval.trim() || '30s'],
    ['VPSMONITOR_REQUEST_TIMEOUT_SECONDS', String(Math.max(1, Number(form.request_timeout_seconds || 15)))],
    ['VPSMONITOR_REALM_AUTO_INSTALL', String(Boolean(form.realm_auto_install) && !haproxyAutoInstall)],
    ['VPSMONITOR_REALM_VERSION', form.realm_version.trim() || 'v2.9.4'],
    ['VPSMONITOR_REALM_DOWNLOAD_BASE_URL', form.realm_download_base_url.trim()],
    ['VPSMONITOR_HAPROXY_AUTO_INSTALL', String(haproxyAutoInstall)],
    ['VPSMONITOR_ASSUME_YES', 'true'],
  ]
  const envText = envValues.map(([key, value]) => `${key}=${shellQuote(value)}`).join(' ')
  const quotedURL = shellQuote(scriptURL)
  return `tmp=/tmp/vpsmonitor-install-openwrt.sh; if command -v uclient-fetch >/dev/null 2>&1; then uclient-fetch -O "$tmp" ${quotedURL}; elif command -v wget >/dev/null 2>&1; then wget -O "$tmp" ${quotedURL}; else curl -fL ${quotedURL} -o "$tmp"; fi && env ${envText} sh "$tmp" client`
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
    return `${psURL}?v=2026072701`
  }
  return psURL
}

export function openWrtInstallScriptURL(scriptURL: string): string {
  const value = (scriptURL || defaultClientInstallCommandForm().install_script_url).trim()
  const match = value.match(/^([^?#]*)([?#].*)?$/)
  const base = match?.[1] || value
  const suffix = match?.[2] || ''
  return base.endsWith('/install.sh') ? `${base.slice(0, -'/install.sh'.length)}/install-openwrt.sh${suffix}` : value
}

export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

export function powerShellQuote(value: string): string {
  return `'${value.replace(/'/g, `''`)}'`
}
