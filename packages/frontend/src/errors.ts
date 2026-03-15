export type ConnectErrorType =
  | 'iframe_blocked'
  | 'session_token_missing'
  | 'already_open'

export class ConnectError extends Error {
  type: ConnectErrorType
  constructor(message: string, type: ConnectErrorType) {
    super(message)
    this.name = 'ConnectError'
    this.type = type
  }
}
