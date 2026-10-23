import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  CheckCircleFilled,
  CloudServerOutlined,
  CustomerServiceOutlined,
  GlobalOutlined,
  ThunderboltFilled,
  WifiOutlined,
} from '@ant-design/icons'
import { Button, Space, Typography } from 'antd'
import { adminEntryPath, withAppBasePath } from '../../lib/appBasePath'

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
          <a className="recommendation-public-brand" href={withAppBasePath('/site')}>
            <span className="recommendation-public-brand-mark">Z</span>
            <span>ZaneLin <small>NETWORK PICKS</small></span>
          </a>
          <Space wrap>
            <Button icon={<ArrowLeftOutlined />} href={withAppBasePath('/site')}>拓扑首页</Button>
            <Button type="primary" href={adminEntryPath()}>进入控制台</Button>
          </Space>
        </header>
      ) : null}

      <main className="recommendation-page">
        <section className="recommendation-hero">
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
          <Button type="primary" size="large" href={withAppBasePath('/site')} icon={<ArrowRightOutlined />}>查看公开拓扑</Button>
        </section>
      </main>
    </div>
  )
}
