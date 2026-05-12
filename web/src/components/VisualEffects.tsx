import { useEffect } from 'react'

import type { FrontendSettings } from '../types'
import { fetchJSON } from '../lib/appHelpers'

const DEFAULT_BACKGROUND_IMAGE = ''
const CUSTOM_EFFECT_LAYER_Z_INDEX = '2147483000'

let customEffectObserver: MutationObserver | null = null

declare global {
  interface Window {
    CustomBackgroundImage?: string
  }
}

export function VisualEffects() {
  useEffect(() => {
    let cancelled = false
    void fetchJSON<FrontendSettings>('/api/v1/frontend-settings')
      .then((settings) => {
        if (!cancelled) {
          applyCustomFrontendCode(settings.custom_code || '')
        }
      })
      .catch(() => {
        if (!cancelled) {
          applyCustomFrontendCode('')
        }
      })
    return () => {
      cancelled = true
      clearCustomFrontendCode()
    }
  }, [])

  return null
}

export function clearCustomFrontendCode() {
  customEffectObserver?.disconnect()
  customEffectObserver = null
  document.querySelectorAll('[data-vpsmonitor-custom-code="true"]').forEach((node) => node.remove())
  document.querySelectorAll('[data-vpsmonitor-generated-effect="true"], #canvas_sakura, .heart').forEach((node) => node.remove())
  delete window.CustomBackgroundImage
  document.documentElement.style.setProperty('--custom-bg-image', 'none')
  applyShellBackground('none')
}

export function applyCustomFrontendCode(customCode: string) {
  clearCustomFrontendCode()
  if (customCode.trim()) {
    prepareCustomEffectContainers()
  }
  const template = document.createElement('template')
  template.innerHTML = customCode
  Array.from(template.content.childNodes).forEach((node) => appendCustomNode(node))
  if (customCode.trim()) {
    installCustomEffectObserver()
    normalizeCustomEffectLayers()
  }
  applyCustomBackgroundImage()
}

function appendCustomNode(node: Node) {
  if (node.nodeType === Node.TEXT_NODE && !node.textContent?.trim()) {
    return
  }
  if (node.nodeName.toLowerCase() === 'script') {
    const source = node as HTMLScriptElement
    const script = document.createElement('script')
    Array.from(source.attributes).forEach((attr) => script.setAttribute(attr.name, attr.value))
    script.text = source.text
    script.dataset.vpsmonitorCustomCode = 'true'
    script.async = false
    document.body.appendChild(script)
    return
  }
  const element = node.cloneNode(true) as HTMLElement
  if (element instanceof HTMLElement) {
    element.dataset.vpsmonitorCustomCode = 'true'
  }
  document.body.appendChild(element)
}

function applyCustomBackgroundImage() {
  const backgroundImage = (window.CustomBackgroundImage || DEFAULT_BACKGROUND_IMAGE).trim()
  const resolvedBackgroundImage = backgroundImage ? proxiedImageURL(backgroundImage) : ''
  const cssURL = resolvedBackgroundImage ? `url("${escapeCSSURL(resolvedBackgroundImage)}")` : 'none'
  document.documentElement.style.setProperty('--custom-bg-image', cssURL)
  applyShellBackground(cssURL)
}

function prepareCustomEffectContainers() {
  if (!document.querySelector('.js-cursor-container')) {
    const cursorContainer = document.createElement('span')
    cursorContainer.className = 'js-cursor-container'
    cursorContainer.dataset.vpsmonitorGeneratedEffect = 'true'
    document.body.appendChild(cursorContainer)
  }
  if (!document.getElementById('canvas_snow')) {
    // Some third-party sakura scripts resize #canvas_snow by mistake.
    // Keeping a hidden compatibility canvas prevents their resize handler from crashing.
    const compatibilityCanvas = document.createElement('canvas')
    compatibilityCanvas.id = 'canvas_snow'
    compatibilityCanvas.dataset.vpsmonitorGeneratedEffect = 'true'
    compatibilityCanvas.width = window.innerWidth
    compatibilityCanvas.height = window.innerHeight
    compatibilityCanvas.style.cssText = 'position:fixed;left:0;top:0;width:0;height:0;opacity:0;pointer-events:none;z-index:-1;'
    document.body.appendChild(compatibilityCanvas)
  }
}

function installCustomEffectObserver() {
  if (!document.body) {
    return
  }
  customEffectObserver = new MutationObserver(() => {
    window.requestAnimationFrame(normalizeCustomEffectLayers)
  })
  customEffectObserver.observe(document.body, { childList: true, subtree: true })
}

function normalizeCustomEffectLayers() {
  document.querySelectorAll<HTMLElement>('#canvas_sakura').forEach((element) => {
    element.style.position = 'fixed'
    element.style.left = '0'
    element.style.top = '0'
    element.style.width = '100vw'
    element.style.height = '100vh'
    element.style.pointerEvents = 'none'
    element.style.zIndex = CUSTOM_EFFECT_LAYER_Z_INDEX
  })
  document.querySelectorAll<HTMLElement>('.js-cursor-container').forEach((element) => {
    element.style.position = 'fixed'
    element.style.inset = '0'
    element.style.pointerEvents = 'none'
    element.style.zIndex = CUSTOM_EFFECT_LAYER_Z_INDEX
  })
  document.querySelectorAll<HTMLElement>('.js-cursor-container > *, .heart').forEach((element) => {
    element.style.pointerEvents = 'none'
    element.style.zIndex = CUSTOM_EFFECT_LAYER_Z_INDEX
  })
}

function escapeCSSURL(value: string) {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '')
}

function proxiedImageURL(value: string) {
  try {
    const url = new URL(value, window.location.href)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      return value
    }
    if (url.origin === window.location.origin) {
      return url.toString()
    }
    return `/api/v1/image-proxy?url=${encodeURIComponent(url.toString())}`
  } catch {
    return value
  }
}

function applyShellBackground(cssURL: string) {
  const background = cssURL === 'none'
    ? ''
    : `var(--custom-bg-overlay), ${cssURL} center / cover fixed no-repeat, var(--page-overlay-strong)`
  document.querySelectorAll<HTMLElement>('.page-shell, .login-shell').forEach((element) => {
    element.style.background = background
  })
}
