import {
  ArrowRightOutlined,
  CheckCircleFilled,
  CloudServerOutlined,
  CustomerServiceOutlined,
  GlobalOutlined,
  SendOutlined,
  ThunderboltFilled,
  WifiOutlined,
  WhatsAppOutlined,
} from '@ant-design/icons'
import { Button, Space, Typography } from 'antd'
import { withAppBasePath } from '../../lib/appBasePath'

const carriers = [
  { name: '电信', detail: 'CN2 GIA', icon: <CloudServerOutlined />, tone: 'blue' },
  { name: '联通', detail: '9929 / 10099', icon: <WifiOutlined />, tone: 'gold' },
  { name: '移动', detail: 'CMI N2', icon: <GlobalOutlined />, tone: 'cyan' },
]

const benefits = [
  {
    title: '人工售后响应',
    description: '遇到线路、设备或使用问题，有人跟进处理。',
    icon: <CustomerServiceOutlined />,
  },
  {
    title: '异常及时跟进',
    description: '持续关注线路状态，出现异常时及时同步和排查。',
    icon: <ThunderboltFilled />,
  },
  {
    title: '线路持续维护',
    description: '只选稳定的 VIP 通道，减少常规线路带来的波动。',
    icon: <CheckCircleFilled />,
  },
]

export function RecommendationsPage({ publicPage = false }: { publicPage?: boolean }) {
  return (
    <div className={`recommendation-page-shell${publicPage ? ' recommendation-page-shell-public' : ''}`}>
      {publicPage ? (
        <header className="recommendation-public-header">
          <a className="recommendation-public-brand" href={withAppBasePath('/recommendations')}>
            <span className="recommendation-public-brand-mark">Z</span>
            <span>ZaneLin <small>NETWORK PICKS</small></span>
          </a>
        </header>
      ) : null}

      <main className="recommendation-page">
        <section className="recommendation-hero" id="recommendation-top">
          <div className="recommendation-hero-copy">
            <div className="recommendation-kicker"><GlobalOutlined /> ZANELIN · PREMIUM NETWORK</div>
            <Typography.Title level={1}>美国精品家宽<br /><em>三网优选</em></Typography.Title>
            <Typography.Paragraph>
              专注美国本土家宽线路，为直播、跨境业务和高质量访问提供稳定的网络体验。
            </Typography.Paragraph>
            <div className="recommendation-speed-lockup" aria-label="最高可达 1.5 Gbps">
              <strong>1.5</strong><span>Gbps</span>
              <small>最高可达 · 晚高峰速度稳定</small>
            </div>
            <div className="recommendation-contact-strip" aria-label="个人联系方式">
              <span className="recommendation-contact-label">直接联系</span>
              <a href="https://t.me/AlanStone1015" target="_blank" rel="noopener noreferrer">
                <SendOutlined /><span>Telegram</span><strong>@AlanStone1015</strong>
              </a>
              <a href="https://wa.me/zanelin1" target="_blank" rel="noopener noreferrer">
                <WhatsAppOutlined /><span>WhatsApp</span><strong>zanelin1</strong>
              </a>
            </div>
            <Space wrap size={10}>
              <Button type="primary" size="large" href="#recommendation-carriers" icon={<ArrowRightOutlined />}>了解三网线路</Button>
              <Button size="large" href="#recommendation-consult">欢迎私信咨询</Button>
            </Space>
          </div>
          <div className="recommendation-network-scene" aria-label="美国三网网络示意图">
            <div className="recommendation-scene-glow" />
            <div className="recommendation-scene-ring recommendation-scene-ring-one" />
            <div className="recommendation-scene-ring recommendation-scene-ring-two" />
            <div className="recommendation-us-orbit">
              <span className="recommendation-us-label">USA</span>
              <small>HOME NETWORK</small>
            </div>
            <div className="recommendation-scene-node recommendation-scene-node-west">AT&amp;T</div>
            <div className="recommendation-scene-node recommendation-scene-node-center">COX</div>
            <div className="recommendation-scene-node recommendation-scene-node-east">Frontier</div>
            <span className="recommendation-scene-line recommendation-scene-line-one" />
            <span className="recommendation-scene-line recommendation-scene-line-two" />
            <span className="recommendation-scene-line recommendation-scene-line-three" />
          </div>
        </section>

        <section className="recommendation-carriers" id="recommendation-carriers">
          <div className="recommendation-section-heading">
            <div>
              <div className="recommendation-section-kicker">三网优化 · VIP 通道</div>
              <Typography.Title level={2}>只选 VIP 通道，不走常规线路</Typography.Title>
            </div>
            <Typography.Text>根据线路和使用场景，为你匹配合适的入口。</Typography.Text>
          </div>
          <div className="recommendation-carrier-grid">
            {carriers.map((carrier) => (
              <div className={`recommendation-carrier-card ${carrier.tone}`} key={carrier.name}>
                <div className="recommendation-carrier-icon">{carrier.icon}</div>
                <div><strong>{carrier.name}</strong><span>{carrier.detail}</span></div>
                <ArrowRightOutlined className="recommendation-carrier-arrow" />
              </div>
            ))}
          </div>
        </section>

        <section className="recommendation-explainers">
          <article className="recommendation-explainer">
            <div className="recommendation-explainer-icon"><ThunderboltFilled /></div>
            <div className="recommendation-section-kicker">OPTIMIZED ROUTE</div>
            <Typography.Title level={2}>优化线路为什么优于普通线路？</Typography.Title>
            <Typography.Paragraph>
              普通线路通常依赖共享的公共转发路径，遇到晚高峰或跨网传输时，更容易出现拥塞、绕路和抖动。优化线路会优先选择更稳定的跨网互联和 VIP 通道，让数据走更合适的路径。
            </Typography.Paragraph>
            <ul>
              <li><CheckCircleFilled /> 更少绕路，延迟和丢包更容易保持稳定</li>
              <li><CheckCircleFilled /> 晚高峰拥塞更少，直播和实时业务体验更平滑</li>
              <li><CheckCircleFilled /> 持续监测线路状态，异常时可以及时切换和排查</li>
            </ul>
            <small className="recommendation-explainer-note">线路质量会受地区、运营商、本地网络和套餐资源影响，优化线路不等于对所有网络环境作绝对保证。</small>
          </article>

          <article className="recommendation-explainer">
            <div className="recommendation-explainer-icon recommendation-explainer-icon-gold"><GlobalOutlined /></div>
            <div className="recommendation-section-kicker">RESIDENTIAL IP FOR AI</div>
            <Typography.Title level={2}>为什么家宽 IP 更适合 AI 工具？</Typography.Title>
            <Typography.Paragraph>
              家宽 IP 通常来自真实家庭网络的运营商 ASN，网络画像更接近普通用户。广播 IP 或机房 IP 往往集中在数据中心，地址复用和异常流量更容易被风控系统识别。
            </Typography.Paragraph>
            <ul>
              <li><CheckCircleFilled /> 用户来源更自然，减少因机房 ASN 触发的额外验证</li>
              <li><CheckCircleFilled /> 区域和网络类型更贴近真实使用场景</li>
              <li><CheckCircleFilled /> 对登录、对话和内容工具的连续使用更友好</li>
            </ul>
            <small className="recommendation-explainer-note">家宽 IP 也不代表一定不会触发验证，仍需遵守 AI 服务商的条款、地区限制和使用政策。</small>
          </article>
        </section>

        <section className="recommendation-benefits">
          <div className="recommendation-section-heading">
            <div>
              <div className="recommendation-section-kicker">WHY ZANELIN</div>
              <Typography.Title level={2}>稳定之外，更重视长期体验</Typography.Title>
            </div>
            <Typography.Text>从线路选择到售后跟进，都有人负责。</Typography.Text>
          </div>
          <div className="recommendation-benefit-grid">
            {benefits.map((benefit) => (
              <article className="recommendation-benefit" key={benefit.title}>
                <div className="recommendation-benefit-icon">{benefit.icon}</div>
                <Typography.Title level={3}>{benefit.title}</Typography.Title>
                <Typography.Paragraph>{benefit.description}</Typography.Paragraph>
              </article>
            ))}
          </div>
        </section>

        <section className="recommendation-consult" id="recommendation-consult">
          <div className="recommendation-consult-mark"><CustomerServiceOutlined /></div>
          <div>
            <div className="recommendation-section-kicker">READY WHEN YOU ARE</div>
            <Typography.Title level={2}>欢迎私信咨询</Typography.Title>
          <Typography.Paragraph>
              我们会根据你的地区、用途和预算，推荐合适的线路。实际速度受套餐、设备及本地网络环境影响。
            </Typography.Paragraph>
          </div>
          <div className="recommendation-consult-actions">
            <Space wrap>
              <Button type="primary" href="https://t.me/AlanStone1015" target="_blank" rel="noopener noreferrer" icon={<SendOutlined />}>Telegram</Button>
              <Button href="https://wa.me/zanelin1" target="_blank" rel="noopener noreferrer" icon={<WhatsAppOutlined />}>WhatsApp</Button>
            </Space>
          </div>
        </section>
      </main>
    </div>
  )
}
