import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Empty, Input, Pagination } from 'antd'
import zhCN from 'antd/es/locale/zh_CN'

import type { XUINodeView } from '../types'
import { nodeElementId } from '../lib/appHelpersAgent'

interface XUINodeListProps {
  nodes: XUINodeView[]
  agentID: string
  selectedNodeAnchor: string
  active: boolean
  children: (nodes: XUINodeView[]) => ReactNode
}

export function XUINodeList({ nodes, agentID, selectedNodeAnchor, active, children }: XUINodeListProps) {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const handledAnchorRef = useRef('')
  const query = search.trim().toLowerCase()
  const filteredNodes = query
    ? nodes.filter((node) => [node.remark, node.tag, node.protocol, node.port, node.id]
        .some((value) => String(value ?? '').toLowerCase().includes(query)))
    : nodes
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filteredNodes.length / pageSize)))
  const visibleNodes = filteredNodes.slice((currentPage - 1) * pageSize, currentPage * pageSize)
  const anchorIndex = nodes.findIndex((node) =>
    nodeElementId(agentID, node.remark || node.tag || `Inbound #${node.id || 0}`) === selectedNodeAnchor)

  // A refreshed snapshot may remove the last page; keep the controls and rows in sync.
  useEffect(() => {
    setPage(currentPage)
  }, [currentPage])

  useEffect(() => {
    if (!active || !selectedNodeAnchor) {
      handledAnchorRef.current = ''
      return
    }
    if (anchorIndex < 0 || handledAnchorRef.current === selectedNodeAnchor) return
    handledAnchorRef.current = selectedNodeAnchor
    setSearch('')
    setPage(Math.floor(anchorIndex / pageSize) + 1)
  }, [active, selectedNodeAnchor, anchorIndex, pageSize])

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
            setPage(1)
          }}
        />
        <Pagination
          current={currentPage}
          pageSize={pageSize}
          total={filteredNodes.length}
          showSizeChanger
          pageSizeOptions={[5, 10, 20, 50]}
          showTotal={(total) => `共 ${total} 个节点`}
          locale={zhCN.Pagination}
          responsive
          style={{ display: 'flex', flexWrap: 'wrap', rowGap: 8 }}
          onChange={(nextPage, nextPageSize) => {
            setPage(nextPageSize === pageSize ? nextPage : 1)
            setPageSize(nextPageSize)
          }}
        />
      </div>
      {visibleNodes.length ? children(visibleNodes) : <Empty description={query ? '未找到匹配的节点' : '暂无 x-ui 节点数据'} />}
    </div>
  )
}
