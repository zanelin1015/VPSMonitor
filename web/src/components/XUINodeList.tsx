import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Empty, Input, Pagination } from 'antd'
import zhCN from 'antd/es/locale/zh_CN'

import type { XUINodeView } from '../types'
import { nodeElementId } from '../lib/appHelpersAgent'
import { listPagination } from '../lib/pagination'
import { useListPagination } from '../hooks/useListPagination'

interface XUINodeListProps {
  nodes: XUINodeView[]
  agentID: string
  selectedNodeAnchor: string
  active: boolean
  children: (nodes: XUINodeView[]) => ReactNode
}

export function XUINodeList({ nodes, agentID, selectedNodeAnchor, active, children }: XUINodeListProps) {
  const [search, setSearch] = useState('')
  const handledAnchorRef = useRef('')
  const query = search.trim().toLowerCase()
  const filteredNodes = query
    ? nodes.filter((node) => [node.remark, node.tag, node.protocol, node.port, node.id]
        .some((value) => String(value ?? '').toLowerCase().includes(query)))
    : nodes
  const { page, pageSize, onChange } = useListPagination(filteredNodes.length, agentID)
  const visibleNodes = filteredNodes.slice((page - 1) * pageSize, page * pageSize)
  const anchorIndex = nodes.findIndex((node) =>
    nodeElementId(agentID, node.remark || node.tag || `Inbound #${node.id || 0}`) === selectedNodeAnchor)

  useEffect(() => {
    if (!active || !selectedNodeAnchor) {
      handledAnchorRef.current = ''
      return
    }
    if (anchorIndex < 0 || handledAnchorRef.current === selectedNodeAnchor) return
    handledAnchorRef.current = selectedNodeAnchor
    setSearch('')
    onChange(Math.floor(anchorIndex / pageSize) + 1)
  }, [active, selectedNodeAnchor, anchorIndex, pageSize, onChange])

  return (
    <div style={{ display: 'grid', gap: 16, minWidth: 0 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
        <Input.Search
          allowClear
          aria-label="搜索节点"
          placeholder="按节点名称、标签、协议或端口搜索"
          style={{ width: '100%', maxWidth: 380 }}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value)
            onChange(1)
          }}
        />
        <Pagination
          {...listPagination}
          current={page}
          pageSize={pageSize}
          total={filteredNodes.length}
          showTotal={(total) => `共 ${total} 个节点`}
          locale={zhCN.Pagination}
          responsive
          style={{ display: 'flex', flexWrap: 'wrap', rowGap: 8 }}
          onChange={onChange}
        />
      </div>
      {visibleNodes.length ? children(visibleNodes) : <Empty description={query ? '未找到匹配的节点' : '暂无 x-ui 节点数据'} />}
    </div>
  )
}
