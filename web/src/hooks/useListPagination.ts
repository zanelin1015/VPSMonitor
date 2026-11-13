import { useCallback, useEffect, useState } from 'react'
import { DEFAULT_PAGE_SIZE } from '../lib/pagination'

export function useListPagination(total: number, resetKey: unknown) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const lastPage = Math.max(1, Math.ceil(total / pageSize))

  useEffect(() => {
    setPage(1)
  }, [resetKey])

  useEffect(() => {
    setPage((current) => Math.min(current, lastPage))
  }, [lastPage])

  const onChange = useCallback((nextPage: number, nextPageSize = pageSize) => {
    setPage(nextPageSize === pageSize ? nextPage : 1)
    setPageSize(nextPageSize)
  }, [pageSize])

  return { page: Math.min(page, lastPage), pageSize, onChange }
}
