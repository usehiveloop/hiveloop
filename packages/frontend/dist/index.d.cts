type ThemeOption = 'light' | 'dark' | 'system';
type ConnectErrorCode = 'session_invalid' | 'session_expired' | 'connection_failed' | 'integration_failed' | 'unknown_error';
type ConnectEvent = {
    type: 'success';
    payload: SuccessPayload;
} | {
    type: 'integration_success';
    payload: IntegrationSuccessPayload;
} | {
    type: 'error';
    payload: ErrorPayload;
} | {
    type: 'close';
};
interface SuccessPayload {
    providerId: string;
    connectionId: string;
}
interface IntegrationSuccessPayload {
    integrationId: string;
    provider: string;
}
interface ErrorPayload {
    code: ConnectErrorCode;
    message: string;
    providerId?: string;
}
interface LLMVaultConnectConfig {
    baseURL?: string;
    theme?: ThemeOption;
}
interface ConnectOpenOptions {
    sessionToken: string;
    onSuccess?: (payload: SuccessPayload) => void;
    onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void;
    onError?: (payload: ErrorPayload) => void;
    onClose?: () => void;
    onEvent?: (event: ConnectEvent) => void;
}

declare class LLMVaultConnect {
    private iframe;
    private listener;
    private baseURL;
    private baseOrigin;
    private theme;
    private options;
    private previousOverflow;
    constructor(config?: LLMVaultConnectConfig);
    open(options: ConnectOpenOptions): void;
    close(): void;
    get isOpen(): boolean;
}

type ConnectErrorType = 'iframe_blocked' | 'session_token_missing' | 'already_open';
declare class ConnectError extends Error {
    type: ConnectErrorType;
    constructor(message: string, type: ConnectErrorType);
}

export { ConnectError, type ConnectErrorCode, type ConnectErrorType, type ConnectEvent, type ConnectOpenOptions, type ErrorPayload, type IntegrationSuccessPayload, LLMVaultConnect, type LLMVaultConnectConfig, type SuccessPayload, type ThemeOption };
