export type ConnectErrorType = 'iframe_blocked' | 'session_token_missing' | 'already_open';
export declare class ConnectError extends Error {
    type: ConnectErrorType;
    constructor(message: string, type: ConnectErrorType);
}
//# sourceMappingURL=errors.d.ts.map