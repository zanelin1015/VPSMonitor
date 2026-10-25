import { useEffect, useState } from 'react'
import { Alert, AutoComplete, Button, Empty, Space, Table, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'

import type { XUILocalCertificate } from '../types'
import type { AgentDetailPanelProps } from './AgentDetailPanel.types'
import { formatDateTime } from '../lib/appHelpers'
import { buildPrimaryDomainOptions, firstCertificateDomainCandidate } from './AgentDetailHelpers'

const { Text } = Typography

type AgentCertificatesPanelProps = Pick<AgentDetailPanelProps, 'selectedAgentId' | 'canManageConfig' | 'configSavingSection' | 'onSavePrimaryDomain'> & {
  certificates?: XUILocalCertificate[]
  currentPrimaryDomain: string
}

export function AgentCertificatesPanel({ certificates, currentPrimaryDomain, selectedAgentId, canManageConfig, configSavingSection, onSavePrimaryDomain }: AgentCertificatesPanelProps) {
  const certificateDomainOptions = buildPrimaryDomainOptions(certificates || [])
  const [primaryDomainDraft, setPrimaryDomainDraft] = useState(currentPrimaryDomain)

  useEffect(() => {
    setPrimaryDomainDraft(currentPrimaryDomain)
  }, [currentPrimaryDomain, selectedAgentId])

  const certificateColumns: ColumnsType<XUILocalCertificate> = [
    {
      title: '证书',
      key: 'certificate',
      width: 240,
      render: (_, record) => (
        <div>
          <Text strong>{record.name || record.subject || record.id}</Text>
          <div className="muted-line">{record.subject || '-'}</div>
        </div>
      ),
    },
    {
      title: '域名',
      dataIndex: 'dns_names',
      width: 280,
      render: (value?: string[]) => value?.length ? value.join(', ') : '-',
    },
    {
      title: '到期时间',
      dataIndex: 'not_after',
      width: 180,
      render: (value?: string) => formatDateTime(value),
    },
    {
      title: '证书路径',
      dataIndex: 'cert_path',
      width: 260,
      render: (value?: string) => value || '-',
    },
    {
      title: '私钥路径',
      dataIndex: 'key_path',
      width: 260,
      render: (value?: string) => value || '-',
    },
    {
      title: '主域名',
      key: 'primary_domain',
      width: 150,
      fixed: 'right',
      render: (_, record) => {
        const domain = firstCertificateDomainCandidate(record)
        if (!domain) {
          return <Text type="secondary">无域名</Text>
        }
        const selected = currentPrimaryDomain === domain
        return (
          <Button
            size="small"
            type={selected ? 'primary' : 'default'}
            disabled={!canManageConfig || configSavingSection === 'entry'}
            loading={configSavingSection === 'entry' && selected}
            onClick={() => onSavePrimaryDomain(domain)}
          >
            {selected ? '当前主域名' : '设为主域名'}
          </Button>
        )
      },
    },
  ]

  return certificates ? (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        type="info"
        showIcon
        message="VPS 主域名"
        description="导出的单节点链接会优先使用这里保存的主域名替代 IP；如果该 VPS 是 Realm 或 HAProxy 入口机，转发后的节点也会使用入口机的主域名和监听端口。SNI、Reality 等目标节点参数保持不变。"
      />
      <Space.Compact style={{ width: '100%' }}>
        <AutoComplete
          allowClear
          value={primaryDomainDraft}
          options={certificateDomainOptions}
          placeholder="可从证书域名选择，也可以手动输入域名"
          onChange={setPrimaryDomainDraft}
          filterOption={(inputValue, option) => String(option?.value || '').toLowerCase().includes(inputValue.toLowerCase())}
          style={{ width: '100%' }}
          disabled={!canManageConfig}
        />
        <Button
          type="primary"
          loading={configSavingSection === 'entry'}
          disabled={!canManageConfig || configSavingSection === 'entry' || primaryDomainDraft.trim() === currentPrimaryDomain}
          onClick={() => onSavePrimaryDomain(primaryDomainDraft)}
        >
          保存主域名
        </Button>
      </Space.Compact>
      <Text type="secondary">
        {currentPrimaryDomain ? `当前主域名：${currentPrimaryDomain}` : '当前未设置主域名；未设置时会继续按证书域名、入口地址、公网 IP 的顺序回退。'}
      </Text>
      <Table rowKey={(record) => record.id} columns={certificateColumns} dataSource={certificates} pagination={{ pageSize: 8, hideOnSinglePage: true }} scroll={{ x: 1360 }} />
    </Space>
  ) : (
    <Empty description="暂无本机证书数据" />
  )

}
