FROM hashicorp/vault:1.21.1

ARG UI_ENABLED=true
ARG CLUSTER_ADDR
ARG API_ADDR
ARG LOG_LEVEL=warn
ARG RAFT_STORAGE_PATH=/vault/data
ARG TLS_CERT_BASE64
ARG TLS_KEY_BASE64
ARG TLS_CERT_PATH=/vault/tls/vault.pem
ARG TLS_KEY_PATH=/vault/tls/vault-key.pem
ARG TLS_DISABLED=false
ARG TLS_CLIENT_CERTS_DISABLED=true
ARG ENV=dev
ARG DEV_ROOT_TOKEN_ID=root

RUN mkdir -p /vault/tls

RUN export TLS_DISABLED=${TLS_DISABLED} && \
  # if [ "$TLS_DISABLED" = "false" ]; then \
  export TLS_CERT_BASE64=${TLS_CERT_BASE64} && \
  export TLS_KEY_BASE64=${TLS_KEY_BASE64} && \
  export TLS_CERT_PATH=${TLS_CERT_PATH} && \
  export TLS_KEY_PATH=${TLS_KEY_PATH} && \
  echo $TLS_CERT_BASE64 | base64 -d > ${TLS_CERT_PATH} && \
  echo $TLS_KEY_BASE64 | base64 -d > ${TLS_KEY_PATH}
# ; \
# fi

COPY config.sh /config.sh

RUN chmod +x /config.sh && \
  export UI_ENABLED=${UI_ENABLED} && \
  export CLUSTER_ADDR=${CLUSTER_ADDR} && \
  export API_ADDR=${API_ADDR} && \
  export LOG_LEVEL=${LOG_LEVEL} && \
  export RAFT_STORAGE_PATH=${RAFT_STORAGE_PATH} && \
  export TLS_CERT_PATH=${TLS_CERT_PATH} && \
  export TLS_KEY_PATH=${TLS_KEY_PATH} && \
  export TLS_DISABLED=${TLS_DISABLED} && \
  export TLS_CLIENT_CERTS_DISABLED=${TLS_CLIENT_CERTS_DISABLED} && \
  /config.sh && \
  mv ./config.hcl /vault/config/config.hcl


CMD if [ "$ENV" = "dev" ]; then \
  VAULT_DEV_LISTEN_ADDRESS="[::]:8200" \
  VAULT_DEV_ROOT_TOKEN_ID=${DEV_ROOT_TOKEN_ID} \
  vault server --dev -log-level=debug; \
  else \
  vault server -config="/vault/config/config.hcl"; \
  fi
