import { App as AntdApp } from 'antd'
import { useState } from 'react'

import type { AdminAuthResponse, AdminUser, SystemInfo } from '../types'
import { fetchJSON, isUnauthorized } from '../lib/appHelpers'

export type AdminAccountForm = {
  current_password: string
  new_username: string
  new_password: string
  confirm_password: string
  avatar_url: string
}

const emptyAccountForm = (): AdminAccountForm => ({
  current_password: '',
  new_username: '',
  new_password: '',
  confirm_password: '',
  avatar_url: '',
})

export function useAdminSession() {
  const { message } = AntdApp.useApp()
  const [sessionLoading, setSessionLoading] = useState(true)
  const [adminUser, setAdminUser] = useState<AdminUser | null>(null)
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [loginForm, setLoginForm] = useState({ username: '', password: '' })
  const [accountModalOpen, setAccountModalOpen] = useState(false)
  const [accountSaving, setAccountSaving] = useState(false)
  const [accountForm, setAccountForm] = useState<AdminAccountForm>(() => emptyAccountForm())

  async function loadSession() {
    setSessionLoading(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/session')
      setAdminUser(data.user)
      setSystemInfo(data.system || null)
      setAccountForm((current) => ({ ...current, new_username: data.user.username, avatar_url: data.user.avatar_url || '' }))
    } catch {
      setAdminUser(null)
      setSystemInfo(null)
    } finally {
      setSessionLoading(false)
    }
  }

  async function login() {
    setLoginLoading(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(loginForm),
      })
      setAdminUser(data.user)
      setSystemInfo(data.system || null)
      setAccountForm((current) => ({ ...current, new_username: data.user.username, avatar_url: data.user.avatar_url || '' }))
      setLoginForm({ username: data.user.username, password: '' })
      message.success('登录成功')
    } catch (error) {
      message.error(error instanceof Error ? error.message : '登录失败')
    } finally {
      setLoginLoading(false)
    }
  }

  async function logoutSession() {
    try {
      await fetchJSON<{ status: string }>('/api/v1/admin/logout', { method: 'POST' })
    } catch {
      // Session cleanup is best-effort; local state is cleared either way.
    }
    setAdminUser(null)
    setSystemInfo(null)
  }

  async function saveAccount() {
    if (accountForm.new_password && accountForm.new_password !== accountForm.confirm_password) {
      message.error('两次输入的新密码不一致')
      return
    }
    setAccountSaving(true)
    try {
      const data = await fetchJSON<AdminAuthResponse>('/api/v1/admin/account', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          current_password: accountForm.current_password,
          new_username: accountForm.new_username,
          new_password: accountForm.new_password,
          avatar_url: accountForm.avatar_url,
        }),
      })
      setAdminUser(data.user)
      setSystemInfo(data.system || systemInfo)
      setAccountForm({
        current_password: '',
        new_username: data.user.username,
        new_password: '',
        confirm_password: '',
        avatar_url: data.user.avatar_url || '',
      })
      setAccountModalOpen(false)
      message.success('管理员账号已更新')
    } catch (error) {
      if (isUnauthorized(error)) {
        setAdminUser(null)
      }
      message.error(error instanceof Error ? error.message : '更新账号失败')
    } finally {
      setAccountSaving(false)
    }
  }

  return {
    accountForm,
    accountModalOpen,
    accountSaving,
    adminUser,
    loginForm,
    loginLoading,
    sessionLoading,
    systemInfo,
    loadSession,
    login,
    logoutSession,
    saveAccount,
    setAccountForm,
    setAccountModalOpen,
    setAdminUser,
    setLoginForm,
    setSessionLoading,
  }
}
