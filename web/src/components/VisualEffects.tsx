import { useEffect } from 'react'

import type { FrontendSettings } from '../types'
import { fetchJSON } from '../lib/appHelpers'

const DEFAULT_BACKGROUND_IMAGE = ''

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
    }
  }, [])

  return null
}

export function applyCustomFrontendCode(customCode: string) {
  document.querySelectorAll('[data-vpsmonitor-custom-code="true"]').forEach((node) => node.remove())
  delete window.CustomBackgroundImage
  const template = document.createElement('template')
  template.innerHTML = customCode
  Array.from(template.content.childNodes).forEach((node) => appendCustomNode(node))
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
  document.documentElement.style.setProperty('--custom-bg-image', backgroundImage ? `url("${escapeCSSURL(backgroundImage)}")` : 'none')
}

function escapeCSSURL(value: string) {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '')
}
