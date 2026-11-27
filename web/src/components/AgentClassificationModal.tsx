import { useState } from 'react'
import { App, Checkbox, Form, Input, Modal, Select, Space } from 'antd'
import type { AgentClassification, DashboardAgentView } from '../types'
import { fetchJSON } from '../lib/api'
import { classificationCountry, purposeGroups, suggestAgentPurposes } from '../lib/agentClassification'

export function AgentClassificationModal({ agents, onClose, onSaved }: {
  agents: DashboardAgentView[]
  onClose: () => void
  onSaved: () => void
}) {
  const single = agents.length === 1 ? agents[0] : undefined
  const { message, modal } = App.useApp()
  const [saving, setSaving] = useState(false)
  const [changePurposes, setChangePurposes] = useState(Boolean(single))
  const [changeLocation, setChangeLocation] = useState(Boolean(single))
  const [purposes, setPurposes] = useState(single?.entry?.classification?.purposes || (single && !single.entry?.classification ? suggestAgentPurposes(single) : []))
  const [purposeMode, setPurposeMode] = useState<'add' | 'remove' | 'replace'>('add')
  const [country, setCountry] = useState(single ? classificationCountry(single) : '')
  const [city, setCity] = useState(single?.entry?.classification?.city || single?.geo?.city || '')

  async function save() {
    const patch: AgentClassification & { purpose_mode?: string } = {}
    if (changePurposes) {
      patch.purposes = purposes
      patch.purpose_mode = single ? 'replace' : purposeMode
    }
    if (changeLocation) {
      patch.country_code = country.trim().toUpperCase()
      patch.city = city.trim()
      if (patch.country_code && !/^[A-Z]{2}$/.test(patch.country_code)) {
        message.warning('请输入两位地区代码，例如 CN、HK 或 US')
        return
      }
    }
    setSaving(true)
    try {
      const endpoint = '/api/v1/admin/agent-classification'
      const request = {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_ids: agents.map(agent => agent.agent_id), patch }),
      }
      if (changePurposes) {
        const impact = await fetchJSON<Array<{ assignment_id: number; customer_name: string; agent_name: string; inbound_id: number }>>(`${endpoint}?preview=1`, request)
        if (impact.length > 0 && !await modal.confirm({
          title: `${impact.length} 条现有授权将从客户页和订阅隐藏`,
          content: <Space direction="vertical" style={{ maxHeight: 280, overflowY: 'auto' }}>
            <span>请先核对这些客户是否已授权到对应的国内入口。</span>
            {impact.map(item => <span key={item.assignment_id}>{item.customer_name} · {item.agent_name} · {item.inbound_id}</span>)}
          </Space>,
          okText: '确认更新分类', cancelText: '返回核对',
        })) return
      }
      await fetchJSON(endpoint, request)
      message.success(`已更新 ${agents.length} 台 Client 的分类`)
      onSaved()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存 Client 分类失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal open title={single ? `Client 分类 · ${single.agent_name || single.agent_id}` : `批量分类 · ${agents.length} 台`}
      onCancel={saving ? undefined : onClose} onOk={() => void save()} confirmLoading={saving}
      okButtonProps={{ disabled: (!changePurposes && !changeLocation) || (!single && changePurposes && purposeMode !== 'replace' && !purposes.length) }} okText="保存" cancelText="取消">
      <Form layout="vertical" disabled={saving}>
        {!single ? <Form.Item><Checkbox checked={changePurposes} onChange={event => setChangePurposes(event.target.checked)}>修改所选 Client 的线路用途</Checkbox></Form.Item> : null}
        {!single && changePurposes ? <Form.Item label="修改方式">
          <Select value={purposeMode} onChange={setPurposeMode} options={[
            { value: 'add', label: '追加所选用途' }, { value: 'remove', label: '移除所选用途' }, { value: 'replace', label: '用所选用途替换' },
          ]} />
        </Form.Item> : null}
        {changePurposes ? <Form.Item label="线路用途">
          <Select mode="multiple" options={purposeGroups} value={purposes} onChange={setPurposes} placeholder="选择一个或多个用途" />
        </Form.Item> : null}
        {!single ? <Form.Item><Checkbox checked={changeLocation} onChange={event => setChangeLocation(event.target.checked)}>设置所选 Client 的部署地区</Checkbox></Form.Item> : null}
        {changeLocation ? <Space wrap align="start">
          <Form.Item label="国家／地区代码"><Input value={country} maxLength={2} onChange={event => setCountry(event.target.value)} placeholder="CN / HK / US" /></Form.Item>
          <Form.Item label="城市"><Input value={city} maxLength={80} onChange={event => setCity(event.target.value)} placeholder="可选" /></Form.Item>
        </Space> : null}
      </Form>
    </Modal>
  )
}
