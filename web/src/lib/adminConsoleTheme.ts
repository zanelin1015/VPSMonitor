export { adminConsoleTheme, consoleCSSVariables, consolePalette } from '../theme/tokens'

export function isInternalConsoleRoute(pathname: string, _search = ''): boolean {
  return pathname !== '/customer' && pathname !== '/recommendations'
}

export function isConsoleThemeRoute(pathname: string, search = ''): boolean {
  return pathname !== '/recommendations'
    && (pathname === '/customer' || isInternalConsoleRoute(pathname, search))
}
