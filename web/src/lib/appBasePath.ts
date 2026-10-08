// The original root deployment and the reverse-proxied /zanelin deployment
// share a bundle. Resolve the base from the current entry, not the hostname.
export function appBasePath(pathname = window.location.pathname): string {
  return /^\/zanelin(?:\/|$)/.test(pathname) ? '/zanelin' : ''
}

export function appPathname(pathname = window.location.pathname): string {
  return pathname.slice(appBasePath(pathname).length).replace(/\/+$/, '') || '/'
}

export function withAppBasePath(path: string, pathname = window.location.pathname): string {
  const base = appBasePath(pathname)
  if (!base || !path.startsWith('/') || path.startsWith('//') || path === base || path.startsWith(`${base}/`)) return path
  return base + path
}

export function adminEntryPath(pathname = window.location.pathname): string {
  return appBasePath(pathname) ? `${appBasePath(pathname)}/monitor` : '/'
}

export function customerEntryURL(): string {
  return window.location.origin + withAppBasePath('/customer')
}
