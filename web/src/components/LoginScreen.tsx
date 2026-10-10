import { Button, Input, Space, Typography } from 'antd'
import { ArrowRightOutlined, LockOutlined } from '@ant-design/icons'

const { Text, Title } = Typography

export interface LoginScreenProps {
  loginForm: { username: string; password: string }
  loginLoading: boolean
  title?: string
  subtitle?: string
  consoleAppearance?: boolean
  audience?: 'admin' | 'customer'
  onChange: (value: { username: string; password: string }) => void
  onLogin: () => void
}

export function LoginScreen({ loginForm, loginLoading, title = 'ZaneLin', subtitle = '管理员登录', consoleAppearance = false, audience = 'admin', onChange, onLogin }: LoginScreenProps) {
  const canLogin = Boolean(loginForm.username && loginForm.password)
  const isCustomer = audience === 'customer'

  return (
    <div className={`${consoleAppearance ? 'login-shell console-login-shell' : 'login-shell'}${isCustomer ? ' customer-login-shell' : ''}`}>
      {consoleAppearance ? <div className="console-login-wordmark"><span>Z</span>ZaneLin<small>{audience === 'customer' ? 'CUSTOMER PORTAL' : 'CONTROL CENTER'}</small></div> : null}
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
        <form onSubmit={(event) => {
          event.preventDefault()
          if (canLogin && !loginLoading) {
            onLogin()
          }
        }}>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div>
            {consoleAppearance ? <label htmlFor="console-login-username">用户名</label> : <Text type="secondary">用户名</Text>}
            <Input
              id="console-login-username"
              autoComplete="username"
              aria-required="true"
              required
              size="large"
              autoFocus
              value={loginForm.username}
              onChange={(event) => onChange({ ...loginForm, username: event.target.value })}
            />
          </div>
          <div>
            {consoleAppearance ? <label htmlFor="console-login-password">密码</label> : <Text type="secondary">密码</Text>}
            <Input.Password
              id="console-login-password"
              autoComplete="current-password"
              aria-required="true"
              required
              size="large"
              value={loginForm.password}
              onChange={(event) => onChange({ ...loginForm, password: event.target.value })}
            />
          </div>
          <Button
            block
            size="large"
            type="primary"
            htmlType="submit"
            icon={consoleAppearance ? <ArrowRightOutlined /> : <LockOutlined />}
            iconPosition={consoleAppearance ? 'end' : 'start'}
            loading={loginLoading}
            disabled={!canLogin}
          >
            登录
          </Button>
          </Space>
        </form>
      </section>
      {consoleAppearance ? <aside className="console-login-scene" aria-hidden="true">
        <div className="console-login-orbit console-login-orbit-one" />
        <div className="console-login-orbit console-login-orbit-two" />
        <div className="console-login-scene-copy">
          <span>{audience === 'customer' ? 'YOUR CONNECTION, SIMPLIFIED.' : 'INFRASTRUCTURE, CONNECTED.'}</span>
          <h2>{audience === 'customer' ? <>连接你的世界，<br />从这里开始。</> : <>每一条链路，<br />尽在掌握。</>}</h2>
          <p>{audience === 'customer' ? '授权链路 · 订阅导入 · 在线支持' : '统一监控 · 节点编排 · 账号与授权'}</p>
        </div>
      </aside> : null}
    </div>
  )
}
