export class ConnectError extends Error {
    type;
    constructor(message, type) {
        super(message);
        this.name = 'ConnectError';
        this.type = type;
    }
}
//# sourceMappingURL=errors.js.map