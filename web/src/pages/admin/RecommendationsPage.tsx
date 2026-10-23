import {
  ArrowRightOutlined,
  ArrowLeftOutlined,
  CheckCircleFilled,
  EnvironmentOutlined,
  FireFilled,
  GlobalOutlined,
  ThunderboltFilled,
  VideoCameraFilled,
} from '@ant-design/icons'
import { Button, Card, Input, Segmented, Space, Tag, Typography } from 'antd'
import { useMemo, useState, type CSSProperties } from 'react'
import { adminEntryPath, withAppBasePath } from '../../lib/appBasePath'

type RecommendationCategory = 'all' | 'streaming' | 'cross-border' | 'nat' | 'budget'

interface Recommendation {
  id: string
  name: string
  eyebrow: string
  country: string
  countryCode: string
  category: Exclude<RecommendationCategory, 'all'>
  categoryLabel: string
  description: string
  price: string
  priceUnit: string
  bandwidth: string
  traffic: string
  latency: string
  score: string
  accent: string
  features: string[]
}

const recommendations: Recommendation[] = [
  {
    id: 'cn-us-stream',
    name: 'CN → US 优化线路',
    eyebrow: '直播 / 视频分发',
    country: '美国 · 洛杉矶',
    countryCode: 'US',
    category: 'streaming',
    categoryLabel: '直播优选',
    description: '适合 TikTok、YouTube 等跨境直播，优先保证上行稳定和晚高峰体验。',
    price: '$9.00',
    priceUnit: ' / 月起',
    bandwidth: '100 Mbps 上行',
    traffic: '不限流量可选',
    latency: 'CN → US 低延迟',
    score: '直播推荐 4.9',
    accent: '#3939fc',
    features: ['CN 入口可接入', '可叠加独享落地', '支持多路推流'],
  },
  {
    id: 'hk-nat',
    name: '香港 NAT 轻量节点',
    eyebrow: '低预算 / 近距离',
    country: '中国香港 · 九龙',
    countryCode: 'HK',
    category: 'nat',
    categoryLabel: 'NAT 方案',
    description: '用较低成本获得稳定的香港出口，适合个人代理、测试环境和轻量业务。',
    price: '$4.50',
    priceUnit: ' / 月起',
    bandwidth: '20–100 Mbps',
    traffic: '500 GB 起',
    latency: 'CN → HK 低延迟',
    score: '性价比 4.7',
    accent: '#0f766e',
    features: ['共享 IPv4 / 独立端口', '可配前置入口', '适合轻量业务'],
  },
  {
    id: 'jp-cross-border',
    name: '日本跨境中转',
    eyebrow: '中转 / 游戏 / API',
    country: '日本 · 东京',
    countryCode: 'JP',
    category: 'cross-border',
    categoryLabel: '跨境中转',
    description: '面向需要亚洲低延迟的链式转发，适合作为 CN 入口到海外落地的中继。',
    price: '$7.90',
    priceUnit: ' / 月起',
    bandwidth: '1 Gbps 端口',
    traffic: '2 TB / 月',
    latency: '亚洲多地优化',
    score: '中转推荐 4.8',
    accent: '#d97706',
    features: ['Realm / HAProxy 可用', '支持链式转发', '多地区落地可选'],
  },
  {
    id: 'us-dedicated',
    name: '美国独享 VPS',
    eyebrow: '长期运行 / 独享资源',
    country: '美国 · 圣何塞',
    countryCode: 'US',
    category: 'budget',
    categoryLabel: '独享资源',
    description: '独享计算资源和公网 IPv4，适合直播主节点、面板和需要持续运行的服务。',
    price: '$8.99',
    priceUnit: ' / 月起',
    bandwidth: '1 Gbps 端口',
    traffic: '3 TB / 月',
    latency: '北美稳定线路',
    score: '长期运行 4.6',
    accent: '#7c3aed',
    features: ['独享公网 IPv4', '可自定义系统', '适合生产环境'],
  },
  {
    id: 'tw-home',
    name: '台湾家宽出口',
    eyebrow: '家宽 / 本地化访问',
    country: '中国台湾 · 台北',
    countryCode: 'TW',
    category: 'cross-border',
    categoryLabel: '家宽出口',
    description: '面向需要本地化访问特征的场景，适合小规模业务和区域内容测试。',
    price: '$8.50',
    priceUnit: ' / 月起',
    bandwidth: '100 Mbps',
    traffic: '按套餐提供',
    latency: '东亚低延迟',
    score: '本地化 4.5',
    accent: '#e11d48',
    features: ['家宽网络特征', '支持区域标签', '可配备用落地'],
  },
  {
    id: 'sg-budget',
    name: '新加坡入门 VPS',
    eyebrow: '亚洲 / 入门首选',
    country: '新加坡 · 新加坡',
    countryCode: 'SG',
    category: 'budget',
    categoryLabel: '入门方案',
    description: '适合个人项目、监控和轻量 API，部署快，预算可控，方便后续扩容。',
    price: '$5.99',
    priceUnit: ' / 月起',
    bandwidth: '500 Mbps 端口',
    traffic: '1 TB / 月',
    latency: '亚洲多地可达',
    score: '入门推荐 4.6',
    accent: '#2563eb',
    features: ['快速开通', '基础防护', '支持升级配置'],
  },
]

const categoryOptions: Array<{ value: RecommendationCategory; label: string }> = [
  { value: 'all', label: '全部方案' },
  { value: 'streaming', label: '直播优选' },
  { value: 'cross-border', label: '跨境中转' },
  { value: 'nat', label: 'NAT 节点' },
  { value: 'budget', label: '低预算 / 独享' },
]

export function RecommendationsPage({ publicPage = false }: { publicPage?: boolean }) {
  const [category, setCategory] = useState<RecommendationCategory>('all')
  const [search, setSearch] = useState('')
  const [selectedID, setSelectedID] = useState('')

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return recommendations.filter((item) => {
      const matchesCategory = category === 'all' || item.category === category
      const haystack = [item.name, item.eyebrow, item.country, item.description, ...item.features].join(' ').toLowerCase()
      return matchesCategory && (!keyword || haystack.includes(keyword))
    })
  }, [category, search])

  return (
    <div className={`recommendation-page-shell${publicPage ? ' recommendation-page-shell-public' : ''}`}>
      {publicPage ? (
        <header className="recommendation-public-header">
          <a className="recommendation-public-brand" href={withAppBasePath('/site')}>
            <span className="recommendation-public-brand-mark">Z</span>
            <span>ZaneLin <small>RECOMMENDATIONS</small></span>
          </a>
          <Space wrap>
            <Button icon={<ArrowLeftOutlined />} href={withAppBasePath('/site')}>拓扑首页</Button>
            <Button type="primary" href={adminEntryPath()}>进入控制台</Button>
          </Space>
        </header>
      ) : null}
      <div className="recommendation-page">
      <section className="recommendation-hero">
        <div className="recommendation-hero-copy">
          <div className="recommendation-kicker"><GlobalOutlined /> ROUTE PICKS / 2026</div>
          <Typography.Title level={1}>找到适合你的 VPS 线路</Typography.Title>
          <Typography.Paragraph>
            按直播、中转、家宽和独享资源筛选方案。把节点位置、带宽与预算放在一起比较，少走弯路。
          </Typography.Paragraph>
          <Space wrap size={10}>
            <Button type="primary" size="large" href="#recommendation-list" icon={<ArrowRightOutlined />}>查看推荐</Button>
            <Button size="large" href="#recommendation-note">查看选择建议</Button>
          </Space>
        </div>
        <div className="recommendation-hero-orbit" aria-hidden>
          <div className="recommendation-orbit-grid" />
          <div className="recommendation-orbit-core"><ThunderboltFilled /><span>ROUTE</span></div>
          <div className="recommendation-orbit-node recommendation-orbit-node-cn">CN</div>
          <div className="recommendation-orbit-node recommendation-orbit-node-us">US</div>
          <div className="recommendation-orbit-node recommendation-orbit-node-asia">ASIA</div>
        </div>
      </section>

      <section className="recommendation-stats" aria-label="推荐页概览">
        <RecommendationStat value="6" label="当前精选方案" />
        <RecommendationStat value="4" label="常用业务场景" />
        <RecommendationStat value="$4.50" label="月付起价" />
        <RecommendationStat value="24/7" label="在线状态参考" />
      </section>

      <section className="recommendation-list" id="recommendation-list">
        <div className="recommendation-section-heading">
          <div>
            <Typography.Title level={2}>精选方案</Typography.Title>
            <Typography.Text type="secondary">根据你的线路和业务场景，先从这里开始比较。</Typography.Text>
          </div>
          <Input.Search
            allowClear
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="搜索地区、场景或方案"
            className="recommendation-search"
          />
        </div>
        <div className="recommendation-filter-row">
          <Segmented
            block
            value={category}
            options={categoryOptions}
            onChange={(value) => setCategory(value as RecommendationCategory)}
          />
          <Typography.Text type="secondary">显示 {filtered.length} / {recommendations.length} 个方案</Typography.Text>
        </div>

        <div className="recommendation-grid">
          {filtered.map((item) => (
            <RecommendationCard
              key={item.id}
              item={item}
              selected={selectedID === item.id}
              onSelect={() => setSelectedID((current) => current === item.id ? '' : item.id)}
            />
          ))}
        </div>
        {!filtered.length ? (
          <Card className="recommendation-empty"><Typography.Text type="secondary">没有匹配的方案，换一个关键词试试。</Typography.Text></Card>
        ) : null}
      </section>

      <section className="recommendation-note" id="recommendation-note">
        <div className="recommendation-note-icon"><VideoCameraFilled /></div>
        <div>
          <Typography.Title level={4}>直播场景怎么选？</Typography.Title>
          <Typography.Paragraph>
            上行比下载更重要：单路 1080p 通常建议预留 8–12 Mbps，并为多路推流和晚高峰留出余量。先选稳定入口，再搭配独享落地。
          </Typography.Paragraph>
        </div>
        <Button type="link" href="#recommendation-list">回到方案 <ArrowRightOutlined /></Button>
      </section>
      </div>
    </div>
  )
}

function RecommendationCard({ item, selected, onSelect }: { item: Recommendation; selected: boolean; onSelect: () => void }) {
  return (
    <Card className={`recommendation-card${selected ? ' selected' : ''}`} style={{ '--recommendation-accent': item.accent } as CSSProperties}>
      <div className="recommendation-card-topline"><Tag color="blue">{item.categoryLabel}</Tag><span>{item.score}</span></div>
      <div className="recommendation-card-title">
        <div className="recommendation-country-mark">{item.countryCode}</div>
        <div><Typography.Title level={3}>{item.name}</Typography.Title><Typography.Text type="secondary">{item.eyebrow}</Typography.Text></div>
      </div>
      <Typography.Paragraph className="recommendation-card-description">{item.description}</Typography.Paragraph>
      <div className="recommendation-card-price"><strong>{item.price}</strong><span>{item.priceUnit}</span></div>
      <div className="recommendation-card-meta">
        <span><EnvironmentOutlined /> {item.country}</span>
        <span><ThunderboltFilled /> {item.bandwidth}</span>
        <span><GlobalOutlined /> {item.traffic}</span>
        <span><FireFilled /> {item.latency}</span>
      </div>
      <div className="recommendation-card-features">
        {item.features.map((feature) => <span key={feature}><CheckCircleFilled /> {feature}</span>)}
      </div>
      <Button type={selected ? 'default' : 'primary'} block onClick={onSelect}>
        {selected ? '已加入比较' : '选择此方案'}
      </Button>
    </Card>
  )
}

function RecommendationStat({ value, label }: { value: string; label: string }) {
  return <div className="recommendation-stat"><strong>{value}</strong><span>{label}</span></div>
}
