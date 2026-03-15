import type {
  LLMVaultConnectConfig,
  ConnectOpenOptions,
  ConnectEvent,
  ThemeOption,
} from './types'
import { ConnectError } from './errors'

const DEFAULT_BASE_URL = 'https://connect.llmvault.dev'

export class LLMVaultConnect {
  private iframe: HTMLIFrameElement | null = null
  private listener: ((event: MessageEvent) => void) | null = null
  private baseURL: string
  private baseOrigin: string
  private theme: ThemeOption
  private options: ConnectOpenOptions | null = null
  private previousOverflow: string = ''

  constructor(config?: LLMVaultConnectConfig) {
    this.baseURL = config?.baseURL ?? DEFAULT_BASE_URL
    this.baseOrigin = new URL(this.baseURL).origin
    this.theme = config?.theme ?? 'system'
  }

  open(options: ConnectOpenOptions): void {
    if (this.iframe) {
      throw new ConnectError('Connect widget is already open', 'already_open')
    }
    if (!options.sessionToken) {
      throw new ConnectError('A session token is required to open the connect widget', 'session_token_missing')
    }

    this.options = options

    const iframe = document.createElement('iframe')
    const url = new URL(this.baseURL)
    url.searchParams.set('session', options.sessionToken)
    url.searchParams.set('theme', this.theme)
    if (options.screen) {
      url.searchParams.set('screen', options.screen)
    }
    if (options.preview) {
      url.searchParams.set('preview', 'true')
    }
    iframe.src = url.toString()
    iframe.id = 'llmvault-connect-iframe'
    iframe.style.position = 'fixed'
    iframe.style.top = '0'
    iframe.style.left = '0'
    iframe.style.width = '100%'
    iframe.style.height = '100%'
    iframe.style.border = 'none'
    iframe.style.zIndex = '9999'

    this.previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.body.appendChild(iframe)
    this.iframe = iframe

    this.listener = (event: MessageEvent) => {
      if (event.origin !== this.baseOrigin) return

      const data = event.data
      if (typeof data !== 'object' || data === null || !data.type) return

      const connectEvent = data as ConnectEvent

      switch (connectEvent.type) {
        case 'success':
          this.options?.onSuccess?.(connectEvent.payload)
          break
        case 'integration_success':
          this.options?.onIntegrationSuccess?.(connectEvent.payload)
          break
        case 'error':
          this.options?.onError?.(connectEvent.payload)
          break
        case 'close': {
          const onClose = this.options?.onClose
          this.close()
          onClose?.()
          break
        }
        default:
          return
      }

      this.options?.onEvent?.(connectEvent)
    }

    window.addEventListener('message', this.listener)
  }

  close(): void {
    if (this.listener) {
      window.removeEventListener('message', this.listener)
      this.listener = null
    }
    if (this.iframe) {
      this.iframe.remove()
      this.iframe = null
    }
    document.body.style.overflow = this.previousOverflow
    this.options = null
  }

  get isOpen(): boolean {
    return this.iframe !== null
  }
}
