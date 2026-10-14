export type NodeImportTarget = 'shadowrocket' | 'generic'

type NodeImportLinks = {
  import_url?: string
  shadowrocket_import_url?: string
}

const storageKey = 'vpsmonitor.node-import-target'

export function nodeImportURL(node: NodeImportLinks, target: NodeImportTarget = 'shadowrocket'): string {
  if (target === 'shadowrocket' && node.shadowrocket_import_url) {
    return node.shadowrocket_import_url
  }
  return node.import_url || ''
}

export function readNodeImportTarget(): NodeImportTarget {
  try {
    return window.localStorage.getItem(storageKey) === 'generic' ? 'generic' : 'shadowrocket'
  } catch {
    return 'shadowrocket'
  }
}

export function saveNodeImportTarget(target: NodeImportTarget): void {
  try {
    window.localStorage.setItem(storageKey, target)
  } catch {
    // Export still works when browser storage is unavailable.
  }
}
