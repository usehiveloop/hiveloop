import { ZiraLoopConnect } from '../src/index'

const SESSION_TOKEN = import.meta.env.VITE_SESSION_TOKEN as string

const log = document.getElementById('log')!

function appendLog(message: string) {
  const time = new Date().toLocaleTimeString()
  const line = `[${time}] ${message}`
  console.log(line)
  log.textContent += line + '\n'
  log.scrollTop = log.scrollHeight
}

if (!SESSION_TOKEN) {
  appendLog('ERROR: No session token. Set VITE_SESSION_TOKEN env var.')
}

const connect = new ZiraLoopConnect({
  baseURL: 'https://connect.dev.ziraloop.com',
})

document.getElementById('open')!.addEventListener('click', () => {
  if (!SESSION_TOKEN) {
    appendLog('Cannot open — no session token.')
    return
  }

  appendLog('Opening connect widget...')

  connect.open({
    sessionToken: SESSION_TOKEN,
    onSuccess: (payload) => {
      appendLog(`SUCCESS: provider=${payload.providerId} connection=${payload.connectionId}`)
    },
    onIntegrationSuccess: (payload) => {
      appendLog(`INTEGRATION SUCCESS: integration=${payload.integrationId} provider=${payload.provider}`)
    },
    onError: (payload) => {
      appendLog(`ERROR: code=${payload.code} message=${payload.message}`)
    },
    onClose: () => {
      appendLog('Widget closed.')
    },
    onEvent: (event) => {
      appendLog(`Event: ${event.type}`)
    },
  })
})
