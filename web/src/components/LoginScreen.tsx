import { Button, Input, Space, Typography } from 'antd'
import { LockOutlined } from '@ant-design/icons'

const { Text, Title } = Typography

export interface LoginScreenProps {
  loginForm: { username: string; password: string }
  loginLoading: boolean
  title?: string
  subtitle?: string
  onChange: (value: { username: string; password: string }) => void
  onLogin: () => void
}

export function LoginScreen({ loginForm, loginLoading, title = 'ZaneLin', subtitle = '管理员登录', onChange, onLogin }: LoginScreenProps) {
  const canLogin = Boolean(loginForm.username && loginForm.password)

  return (
    <div className="login-shell">
      <section className="login-panel">
        <div className="login-brand">
          <div className="login-mark">
            <LockOutlined />
          </div>
          <div>
            <Title level={2}>{title}</Title>
            {subtitle ? <Text type="secondary">{subtitle}</Text> : null}
          </div>
        </div>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            <Text type="secondary">用户名</Text>
            <Input
              size="large"
              autoFocus
              value={loginForm.username}
              onChange={(event) => onChange({ ...loginForm, username: event.target.value })}
              onPressEnter={() => {
                if (canLogin) {
                  onLogin()
                }
              }}
            />
          </div>
          <div>
            <Text type="secondary">密码</Text>
            <Input.Password
              size="large"
              value={loginForm.password}
              onChange={(event) => onChange({ ...loginForm, password: event.target.value })}
              onPressEnter={() => {
                if (canLogin) {
                  onLogin()
                }
              }}
            />
          </div>
          <Button
            block
            size="large"
            type="primary"
            icon={<LockOutlined />}
            loading={loginLoading}
            disabled={!canLogin}
            onClick={onLogin}
          >
            登录
          </Button>
        </Space>
      </section>
    </div>
  )
}
