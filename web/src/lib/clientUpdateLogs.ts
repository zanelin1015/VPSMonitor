import type { ClientUpdateLog } from '../types'

export function clientUpdateResult(log: ClientUpdateLog) {
  const forcePrefix = log.force ? '强制升级：' : ''
  if (log.decision === 'skipped') return { key: 'skipped', label: log.force ? '强制已跳过' : '已跳过', color: 'default', detail: `${forcePrefix}${log.reason}` }
  if (log.decision === 'failed') return { key: 'failed', label: '下发失败', color: 'red', detail: log.reason }
  if (log.task_status === 'failed') {
    const uncertain = (log.task_error || '').includes('execution lease expired')
    return { key: 'failed', label: uncertain ? '结果未确认' : '执行失败', color: uncertain ? 'orange' : 'red', detail: log.task_error || 'Client 执行失败，未返回具体原因' }
  }
  if (log.confirmed_at) return { key: 'confirmed', label: '升级成功', color: 'blue', detail: `Client 已重新上报目标版本 v${log.target_version}` }
  if (log.task_status === 'succeeded') return { key: 'dispatched', label: '已启动，待确认', color: 'orange', detail: 'Client 已启动升级程序；等待重启后上报目标版本，不代表安装已完成' }
  if (log.task_status === 'running') return { key: 'dispatched', label: '执行中', color: 'blue', detail: 'Client 已领取升级任务，等待执行回执' }
  return { key: 'dispatched', label: log.force ? '强制待领取' : '待领取', color: 'blue', detail: `${forcePrefix}${log.reason}` }
}

export function filterClientUpdateLogs(logs: ClientUpdateLog[], query: string, result: string) {
  const needle = query.trim().toLowerCase()
  return logs.filter((log) => (result === 'all' || clientUpdateResult(log).key === result) &&
    `${log.agent_name} ${log.agent_id} ${log.version} ${log.target_version} ${log.reason} ${log.task_error || ''} ${log.batch_id}`.toLowerCase().includes(needle))
}
