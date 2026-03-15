import type { LLMVaultConnectConfig, ConnectOpenOptions } from './types';
export declare class LLMVaultConnect {
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
//# sourceMappingURL=connect.d.ts.map