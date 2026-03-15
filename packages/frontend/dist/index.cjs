"use strict";
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/index.ts
var index_exports = {};
__export(index_exports, {
  ConnectError: () => ConnectError,
  LLMVaultConnect: () => LLMVaultConnect
});
module.exports = __toCommonJS(index_exports);

// src/errors.ts
var ConnectError = class extends Error {
  type;
  constructor(message, type) {
    super(message);
    this.name = "ConnectError";
    this.type = type;
  }
};

// src/connect.ts
var DEFAULT_BASE_URL = "https://connect.llmvault.dev";
var LLMVaultConnect = class {
  iframe = null;
  listener = null;
  baseURL;
  baseOrigin;
  theme;
  options = null;
  previousOverflow = "";
  constructor(config) {
    this.baseURL = config?.baseURL ?? DEFAULT_BASE_URL;
    this.baseOrigin = new URL(this.baseURL).origin;
    this.theme = config?.theme ?? "system";
  }
  open(options) {
    if (this.iframe) {
      throw new ConnectError("Connect widget is already open", "already_open");
    }
    if (!options.sessionToken) {
      throw new ConnectError("A session token is required to open the connect widget", "session_token_missing");
    }
    this.options = options;
    const iframe = document.createElement("iframe");
    const url = new URL(this.baseURL);
    url.searchParams.set("session", options.sessionToken);
    url.searchParams.set("theme", this.theme);
    iframe.src = url.toString();
    iframe.id = "llmvault-connect-iframe";
    iframe.style.position = "fixed";
    iframe.style.top = "0";
    iframe.style.left = "0";
    iframe.style.width = "100%";
    iframe.style.height = "100%";
    iframe.style.border = "none";
    iframe.style.zIndex = "9999";
    this.previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.body.appendChild(iframe);
    this.iframe = iframe;
    this.listener = (event) => {
      if (event.origin !== this.baseOrigin) return;
      const data = event.data;
      if (typeof data !== "object" || data === null || !data.type) return;
      const connectEvent = data;
      switch (connectEvent.type) {
        case "success":
          this.options?.onSuccess?.(connectEvent.payload);
          break;
        case "integration_success":
          this.options?.onIntegrationSuccess?.(connectEvent.payload);
          break;
        case "error":
          this.options?.onError?.(connectEvent.payload);
          break;
        case "close": {
          const onClose = this.options?.onClose;
          this.close();
          onClose?.();
          break;
        }
        default:
          return;
      }
      this.options?.onEvent?.(connectEvent);
    };
    window.addEventListener("message", this.listener);
  }
  close() {
    if (this.listener) {
      window.removeEventListener("message", this.listener);
      this.listener = null;
    }
    if (this.iframe) {
      this.iframe.remove();
      this.iframe = null;
    }
    document.body.style.overflow = this.previousOverflow;
    this.options = null;
  }
  get isOpen() {
    return this.iframe !== null;
  }
};
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  ConnectError,
  LLMVaultConnect
});
//# sourceMappingURL=index.cjs.map