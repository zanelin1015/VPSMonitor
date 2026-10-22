export { adminConsoleTheme, consolePalette, consoleCSSVariables } from '../theme/tokens'

export function isInternalConsoleRoute(pathname: string, search = ''): boolean {
  return pathname !== '/customer' && pathname !== '/site' && pathname !== '/official'
    && new URLSearchParams(search).get('page') !== 'site'
}

export function isConsoleThemeRoute(pathname: string, search = ''): boolean {
  // The public marketing site keeps its independent visual identity.
  return new URLSearchParams(search).get('page') !== 'site'
    && (pathname === '/customer' || isInternalConsoleRoute(pathname, search))
}
