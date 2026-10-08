import type { ThemeConfig } from 'antd'

// Adapted from NexKr's public design tokens (https://nexkr.sh/, 2026-10-08).
// No third-party stylesheet, font, script, or brand asset is loaded at runtime.
export const consolePalette = {
  light: {
    accent: '#3939fc', background: '#efefee', surface: '#f7f7f6', raised: '#ffffff',
    sunken: '#e7e7e6', text: '#272727', secondary: '#55555a', muted: '#6c6c75',
    border: '#dededd', success: '#178753', warning: '#b66d00', error: '#c93630',
  },
  dark: {
    accent: '#9090ff', background: '#0d0d0f', surface: '#16161a', raised: '#1e1e23',
    sunken: '#121216', text: '#ededf0', secondary: '#b3b3bc', muted: '#9999a6',
    border: '#34343d', success: '#35c07f', warning: '#f0a02a', error: '#ff6b63',
  },
} as const

export function adminConsoleTheme(dark: boolean, reducedMotion = false): ThemeConfig {
  const palette = dark ? consolePalette.dark : consolePalette.light
  return {
    token: {
      colorPrimary: palette.accent,
      colorInfo: palette.accent,
      colorSuccess: palette.success,
      colorWarning: palette.warning,
      colorError: palette.error,
      colorBgLayout: palette.background,
      colorBgContainer: palette.raised,
      colorBgElevated: palette.raised,
      colorFillAlter: palette.surface,
      colorFillTertiary: palette.sunken,
      colorText: palette.text,
      colorTextSecondary: palette.secondary,
      colorTextTertiary: palette.muted,
      colorTextQuaternary: palette.muted,
      colorBorder: palette.border,
      colorBorderSecondary: palette.border,
      borderRadius: 10,
      borderRadiusLG: 20,
      borderRadiusSM: 7,
      controlHeight: 34,
      controlHeightSM: 26,
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Noto Sans SC", sans-serif',
      fontFamilyCode: 'ui-monospace, "SFMono-Regular", Consolas, monospace',
      boxShadow: dark ? '0 18px 50px rgba(0,0,0,.45)' : '0 18px 50px rgba(39,39,39,.12), 0 4px 12px rgba(39,39,39,.06)',
      boxShadowSecondary: dark ? '0 4px 16px rgba(0,0,0,.3)' : '0 4px 16px rgba(39,39,39,.08), 0 1px 3px rgba(39,39,39,.04)',
      motion: !reducedMotion,
      motionDurationFast: '0.14s',
      motionDurationMid: '0.24s',
      motionDurationSlow: '0.42s',
      motionEaseInOut: 'cubic-bezier(0.4, 0, 0.2, 1)',
      motionEaseOut: 'cubic-bezier(0.16, 1, 0.3, 1)',
    },
    components: {
      Button: {
        fontWeight: 600, primaryShadow: 'none', defaultShadow: 'none', dangerShadow: 'none',
        primaryColor: dark ? palette.background : '#ffffff',
        dangerColor: dark ? palette.background : '#ffffff',
      },
      Card: { headerBg: 'transparent', headerFontSize: 14 },
      Input: { activeBg: palette.raised, hoverBg: palette.surface, activeShadow: `0 0 0 3px ${dark ? 'rgba(111,111,255,.2)' : 'rgba(57,57,252,.12)'}` },
      InputNumber: { activeBg: palette.raised, activeShadow: `0 0 0 3px ${dark ? 'rgba(111,111,255,.2)' : 'rgba(57,57,252,.12)'}` },
      Modal: { contentBg: palette.raised, headerBg: palette.raised, titleFontSize: 18 },
      Table: {
        headerBg: palette.surface, headerColor: palette.secondary,
        rowHoverBg: dark ? '#26262d' : '#f3f3fd', borderColor: palette.border,
        cellPaddingBlock: 12, cellPaddingInline: 14,
        cellPaddingBlockMD: 10, cellPaddingInlineMD: 12,
        cellPaddingBlockSM: 8, cellPaddingInlineSM: 10,
      },
      Tabs: { horizontalItemGutter: 26 },
      Tag: { defaultBg: palette.surface, defaultColor: palette.secondary },
      Segmented: { trackBg: palette.sunken, itemSelectedBg: palette.raised },
    },
  }
}

export function isInternalConsoleRoute(pathname: string, search = ''): boolean {
  return pathname !== '/customer' && pathname !== '/site' && pathname !== '/official'
    && new URLSearchParams(search).get('page') !== 'site'
}

export function isConsoleThemeRoute(pathname: string, search = ''): boolean {
  // The public marketing site keeps its independent visual identity.
  return new URLSearchParams(search).get('page') !== 'site'
    && (pathname === '/customer' || isInternalConsoleRoute(pathname, search))
}
