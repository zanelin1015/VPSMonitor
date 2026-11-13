export const DEFAULT_PAGE_SIZE = 10
export const PAGE_SIZE_OPTIONS = [10, 20, 50]

export const listPagination = {
  defaultPageSize: DEFAULT_PAGE_SIZE,
  pageSizeOptions: PAGE_SIZE_OPTIONS,
  showSizeChanger: {
    'aria-label': '每页条数',
    showSearch: false,
    getPopupContainer: (trigger: HTMLElement) => trigger.ownerDocument.body,
  },
  showLessItems: true,
}
