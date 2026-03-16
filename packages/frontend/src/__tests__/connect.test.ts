import { LLMVaultConnect } from '../connect'
import { ConnectError } from '../errors'

const DEFAULT_ORIGIN = 'https://connect.llmvault.dev'

function getIframe(): HTMLIFrameElement {
  return document.getElementById('llmvault-connect-iframe') as HTMLIFrameElement
}

function dispatchMessage(data: unknown, origin: string = DEFAULT_ORIGIN) {
  window.dispatchEvent(
    new MessageEvent('message', { data, origin })
  )
}

describe('LLMVaultConnect', () => {
  let connect: LLMVaultConnect

  afterEach(() => {
    const iframe = getIframe()
    if (iframe) {
      iframe.remove()
    }
    document.body.style.overflow = ''
  })

  describe('constructor', () => {
    it('default baseURL is https://connect.llmvault.dev', () => {
      connect = new LLMVaultConnect()
      connect.open({ sessionToken: 'tok_test', onClose: () => {} })
      const iframe = getIframe()
      expect(iframe.src).toContain('https://connect.llmvault.dev')
      connect.close()
    })

    it('custom baseURL stores correctly', () => {
      connect = new LLMVaultConnect({ baseURL: 'https://custom.example.com' })
      connect.open({ sessionToken: 'tok_test', onClose: () => {} })
      const iframe = getIframe()
      expect(iframe.src).toContain('https://custom.example.com')
      connect.close()
    })

    it('default theme is system', () => {
      connect = new LLMVaultConnect()
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('theme')).toBe('system')
      connect.close()
    })

    it('custom theme stores correctly', () => {
      connect = new LLMVaultConnect({ theme: 'dark' })
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('theme')).toBe('dark')
      connect.close()
    })
  })

  describe('open()', () => {
    beforeEach(() => {
      connect = new LLMVaultConnect()
    })

    afterEach(() => {
      connect.close()
    })

    it('creates iframe with correct src URL including session and theme params', () => {
      connect.open({ sessionToken: 'my_session_token' })
      const iframe = getIframe()
      expect(iframe).not.toBeNull()
      const url = new URL(iframe.src)
      expect(url.origin).toBe('https://connect.llmvault.dev')
      expect(url.searchParams.get('session')).toBe('my_session_token')
      expect(url.searchParams.get('theme')).toBe('system')
    })

    it('passes screen param when provided', () => {
      connect.open({ sessionToken: 'tok_test', screen: 'connected-list' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('screen')).toBe('connected-list')
    })

    it('does not set screen param when omitted', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.has('screen')).toBe(false)
    })

    it('passes preview param when true', () => {
      connect.open({ sessionToken: 'tok_test', preview: true })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('preview')).toBe('true')
    })

    it('does not set preview param when omitted', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.has('preview')).toBe(false)
    })

    it('does not set preview param when false', () => {
      connect.open({ sessionToken: 'tok_test', preview: false })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.has('preview')).toBe(false)
    })

    it('passes providerId param when provided', () => {
      connect.open({ sessionToken: 'tok_test', screen: 'provider-connect', providerId: 'anthropic' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('screen')).toBe('provider-connect')
      expect(url.searchParams.get('providerId')).toBe('anthropic')
    })

    it('does not set providerId param when omitted', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.has('providerId')).toBe(false)
    })

    it('passes integrationId param when provided', () => {
      connect.open({ sessionToken: 'tok_test', screen: 'integration-connect', integrationId: 'int_slack' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.get('screen')).toBe('integration-connect')
      expect(url.searchParams.get('integrationId')).toBe('int_slack')
    })

    it('does not set integrationId param when omitted', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      const url = new URL(iframe.src)
      expect(url.searchParams.has('integrationId')).toBe(false)
    })

    it('iframe has correct styles', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      expect(iframe.style.position).toBe('fixed')
      expect(iframe.style.top).toBe('0px')
      expect(iframe.style.left).toBe('0px')
      expect(iframe.style.width).toBe('100%')
      expect(iframe.style.height).toBe('100%')
      expect(iframe.style.borderStyle).toBe('none')
      expect(iframe.style.zIndex).toBe('9999')
    })

    it('appends iframe to document.body', () => {
      connect.open({ sessionToken: 'tok_test' })
      const iframe = getIframe()
      expect(iframe.parentElement).toBe(document.body)
    })

    it('sets body overflow to hidden', () => {
      document.body.style.overflow = 'auto'
      connect.open({ sessionToken: 'tok_test' })
      expect(document.body.style.overflow).toBe('hidden')
    })

    it('sets isOpen to true', () => {
      expect(connect.isOpen).toBe(false)
      connect.open({ sessionToken: 'tok_test' })
      expect(connect.isOpen).toBe(true)
    })

    it('throws ConnectError with type already_open if called twice', () => {
      connect.open({ sessionToken: 'tok_test' })
      expect(() => connect.open({ sessionToken: 'tok_test' })).toThrowError(ConnectError)
      try {
        connect.open({ sessionToken: 'tok_test' })
      } catch (e) {
        expect(e).toBeInstanceOf(ConnectError)
        expect((e as ConnectError).type).toBe('already_open')
      }
    })

    it('throws ConnectError with type session_token_missing if sessionToken is empty string', () => {
      expect(() => connect.open({ sessionToken: '' })).toThrowError(ConnectError)
      try {
        connect.open({ sessionToken: '' })
      } catch (e) {
        expect(e).toBeInstanceOf(ConnectError)
        expect((e as ConnectError).type).toBe('session_token_missing')
      }
    })

    it('throws ConnectError with type session_token_missing if sessionToken is undefined', () => {
      expect(() =>
        connect.open({ sessionToken: undefined as unknown as string })
      ).toThrowError(ConnectError)
      try {
        connect.open({ sessionToken: undefined as unknown as string })
      } catch (e) {
        expect(e).toBeInstanceOf(ConnectError)
        expect((e as ConnectError).type).toBe('session_token_missing')
      }
    })
  })

  describe('messages', () => {
    beforeEach(() => {
      connect = new LLMVaultConnect()
    })

    afterEach(() => {
      connect.close()
    })

    it('ignores messages from wrong origin', () => {
      const onSuccess = vi.fn()
      const onError = vi.fn()
      const onClose = vi.fn()
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onSuccess, onError, onClose, onEvent })

      dispatchMessage({ type: 'success', payload: { providerId: 'p1', connectionId: 'c1' } }, 'https://evil.com')

      expect(onSuccess).not.toHaveBeenCalled()
      expect(onError).not.toHaveBeenCalled()
      expect(onClose).not.toHaveBeenCalled()
      expect(onEvent).not.toHaveBeenCalled()
    })

    it('ignores non-object message data', () => {
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onEvent })

      dispatchMessage('some string')
      dispatchMessage(42)
      dispatchMessage(true)

      expect(onEvent).not.toHaveBeenCalled()
    })

    it('ignores messages with missing type field', () => {
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onEvent })

      dispatchMessage({ payload: 'no type' })

      expect(onEvent).not.toHaveBeenCalled()
    })

    it('ignores null message data', () => {
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onEvent })

      dispatchMessage(null)

      expect(onEvent).not.toHaveBeenCalled()
    })

    it('calls onSuccess callback with correct payload', () => {
      const onSuccess = vi.fn()
      connect.open({ sessionToken: 'tok_test', onSuccess })

      const payload = { providerId: 'openai', connectionId: 'conn_123' }
      dispatchMessage({ type: 'success', payload })

      expect(onSuccess).toHaveBeenCalledOnce()
      expect(onSuccess).toHaveBeenCalledWith(payload)
    })

    it('calls onIntegrationSuccess with correct payload', () => {
      const onIntegrationSuccess = vi.fn()
      connect.open({ sessionToken: 'tok_test', onIntegrationSuccess })

      const payload = { integrationId: 'int_456' }
      dispatchMessage({ type: 'integration_success', payload })

      expect(onIntegrationSuccess).toHaveBeenCalledOnce()
      expect(onIntegrationSuccess).toHaveBeenCalledWith(payload)
    })

    it('calls onResourceSelection callback with correct payload', () => {
      const onResourceSelection = vi.fn()
      connect.open({ sessionToken: 'tok_test', onResourceSelection })

      const payload = { integrationId: 'int_789', resources: { channels: ['C001', 'C002'] } }
      dispatchMessage({ type: 'resource_selection', payload })

      expect(onResourceSelection).toHaveBeenCalledOnce()
      expect(onResourceSelection).toHaveBeenCalledWith(payload)
    })

    it('calls onError callback with correct payload', () => {
      const onError = vi.fn()
      connect.open({ sessionToken: 'tok_test', onError })

      const payload = { code: 'connection_failed' as const, message: 'Something went wrong', providerId: 'openai' }
      dispatchMessage({ type: 'error', payload })

      expect(onError).toHaveBeenCalledOnce()
      expect(onError).toHaveBeenCalledWith(payload)
    })

    it('on close event, calls onClose callback and removes iframe', () => {
      const onClose = vi.fn()
      connect.open({ sessionToken: 'tok_test', onClose })

      expect(getIframe()).not.toBeNull()

      dispatchMessage({ type: 'close' })

      expect(onClose).toHaveBeenCalledOnce()
      expect(getIframe()).toBeNull()
      expect(connect.isOpen).toBe(false)
    })

    it('calls onEvent for every recognized event type', () => {
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onEvent })

      dispatchMessage({ type: 'success', payload: { providerId: 'p', connectionId: 'c' } })
      expect(onEvent).toHaveBeenCalledWith({ type: 'success', payload: { providerId: 'p', connectionId: 'c' } })

      dispatchMessage({ type: 'integration_success', payload: { integrationId: 'i' } })
      expect(onEvent).toHaveBeenCalledWith({ type: 'integration_success', payload: { integrationId: 'i' } })

      dispatchMessage({ type: 'resource_selection', payload: { integrationId: 'i', resources: { repos: ['r1'] } } })
      expect(onEvent).toHaveBeenCalledWith({ type: 'resource_selection', payload: { integrationId: 'i', resources: { repos: ['r1'] } } })

      dispatchMessage({ type: 'error', payload: { code: 'unknown_error', message: 'err' } })
      expect(onEvent).toHaveBeenCalledWith({ type: 'error', payload: { code: 'unknown_error', message: 'err' } })

      expect(onEvent).toHaveBeenCalledTimes(4)
    })

    it('does not call onEvent for unrecognized event types', () => {
      const onEvent = vi.fn()
      connect.open({ sessionToken: 'tok_test', onEvent })

      dispatchMessage({ type: 'unknown_type' })

      expect(onEvent).not.toHaveBeenCalled()
    })
  })

  describe('close()', () => {
    beforeEach(() => {
      connect = new LLMVaultConnect()
    })

    it('removes iframe from DOM', () => {
      connect.open({ sessionToken: 'tok_test' })
      expect(getIframe()).not.toBeNull()
      connect.close()
      expect(getIframe()).toBeNull()
    })

    it('restores body overflow to previous value', () => {
      document.body.style.overflow = 'scroll'
      connect.open({ sessionToken: 'tok_test' })
      expect(document.body.style.overflow).toBe('hidden')
      connect.close()
      expect(document.body.style.overflow).toBe('scroll')
    })

    it('removes event listener', () => {
      const removeEventListenerSpy = vi.spyOn(window, 'removeEventListener')
      connect.open({ sessionToken: 'tok_test' })
      connect.close()
      expect(removeEventListenerSpy).toHaveBeenCalledWith('message', expect.any(Function))
      removeEventListenerSpy.mockRestore()
    })

    it('sets isOpen to false', () => {
      connect.open({ sessionToken: 'tok_test' })
      expect(connect.isOpen).toBe(true)
      connect.close()
      expect(connect.isOpen).toBe(false)
    })

    it('no-op when not open and does not throw', () => {
      expect(() => connect.close()).not.toThrow()
      expect(connect.isOpen).toBe(false)
    })
  })

  describe('edge cases', () => {
    beforeEach(() => {
      connect = new LLMVaultConnect()
    })

    afterEach(() => {
      connect.close()
    })

    it('reusable after close: open, close, open works', () => {
      connect.open({ sessionToken: 'tok_first' })
      expect(connect.isOpen).toBe(true)
      expect(getIframe()).not.toBeNull()

      connect.close()
      expect(connect.isOpen).toBe(false)
      expect(getIframe()).toBeNull()

      connect.open({ sessionToken: 'tok_second' })
      expect(connect.isOpen).toBe(true)
      const secondIframe = getIframe()
      expect(secondIframe).not.toBeNull()
      const url = new URL(secondIframe.src)
      expect(url.searchParams.get('session')).toBe('tok_second')
    })

    it('onSuccess is called and listener remains attached for subsequent events', () => {
      const callOrder: string[] = []
      const onSuccess = vi.fn(() => callOrder.push('success'))
      const onClose = vi.fn(() => callOrder.push('close'))
      connect.open({ sessionToken: 'tok_test', onSuccess, onClose })

      dispatchMessage({ type: 'success', payload: { providerId: 'p', connectionId: 'c' } })
      expect(onSuccess).toHaveBeenCalledOnce()

      dispatchMessage({ type: 'close' })
      expect(onClose).toHaveBeenCalledOnce()
      expect(callOrder).toEqual(['success', 'close'])
    })

    it('custom baseURL origin is used for message validation', () => {
      const customConnect = new LLMVaultConnect({ baseURL: 'https://my-vault.example.com/connect' })
      const onSuccess = vi.fn()
      customConnect.open({ sessionToken: 'tok_test', onSuccess })

      dispatchMessage(
        { type: 'success', payload: { providerId: 'p', connectionId: 'c' } },
        'https://connect.llmvault.dev'
      )
      expect(onSuccess).not.toHaveBeenCalled()

      dispatchMessage(
        { type: 'success', payload: { providerId: 'p', connectionId: 'c' } },
        'https://my-vault.example.com'
      )
      expect(onSuccess).toHaveBeenCalledOnce()

      customConnect.close()
    })

    it('preserves empty string as previous overflow value', () => {
      document.body.style.overflow = ''
      connect.open({ sessionToken: 'tok_test' })
      expect(document.body.style.overflow).toBe('hidden')
      connect.close()
      expect(document.body.style.overflow).toBe('')
    })

    it('close event removes listener so subsequent messages are ignored', () => {
      const onSuccess = vi.fn()
      connect.open({ sessionToken: 'tok_test', onSuccess })

      dispatchMessage({ type: 'close' })

      dispatchMessage({ type: 'success', payload: { providerId: 'p', connectionId: 'c' } })
      expect(onSuccess).not.toHaveBeenCalled()
    })
  })
})
