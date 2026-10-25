import { useEffect, useState } from 'react'

export function useListPagination(total: number, resetKey: string) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => {
    setPage(1)
  }, [resetKey])

  useEffect(() => {
    setPage((current) => Math.min(current, Math.max(1, Math.ceil(total / pageSize))))
  }, [total, pageSize])

  function onChange(nextPage: number, nextPageSize = pageSize) {
    setPageSize(nextPageSize)
    setPage(nextPageSize !== pageSize ? 1 : nextPage)
  }

  return { page, pageSize, onChange }
}
