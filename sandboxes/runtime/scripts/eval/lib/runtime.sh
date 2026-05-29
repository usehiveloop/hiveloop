#!/usr/bin/env bash

set -euo pipefail

build_runtime_image() {
  echo -e "${BLUE}[1/7] Building specialist runtime image...${NC}"
  echo "  Image:      $IMAGE"
  echo "  Dockerfile: ${HIVY_SANDBOXES_RUNTIME_DOCKERFILE:-Dockerfile.specialist}"
  echo "  Rebuild:    image=$EVAL_REBUILD_IMAGE binary=$EVAL_REBUILD_BINARY"

  if [[ "$EVAL_REBUILD_IMAGE" == "1" ]] || ! docker image inspect "$IMAGE" &>/dev/null; then
    if [[ "$EVAL_REBUILD_BINARY" == "1" ]] || ! compgen -G "$ROOT/dist/hivy-sandboxes-runtime-*" >/dev/null; then
      echo "  Building Linux release binary from current source..."
      "$ROOT/scripts/build_linux_release.sh"
    else
      echo "  Reusing existing Linux release binary from dist/."
    fi

    HIVY_SANDBOXES_RUNTIME_IMAGE="$IMAGE" \
      HIVY_SANDBOXES_RUNTIME_DOCKERFILE="${HIVY_SANDBOXES_RUNTIME_DOCKERFILE:-Dockerfile.specialist}" \
      "$ROOT/scripts/build_runtime_image.sh"
  else
    echo -e "  ${GREEN}Image exists: $IMAGE${NC}"
  fi

  verify_runtime_image
}

start_eval_container() {
  echo -e "${BLUE}[2/7] Starting container...${NC}"

  if docker ps -a --format '{{.Names}}' | grep -Fxq "$CONTAINER"; then
    if [[ "$EVAL_REPLACE_CONTAINER" == "1" ]]; then
      echo "  Replacing existing container: $CONTAINER"
      docker rm -f "$CONTAINER" >/dev/null
    else
      echo -e "  ${RED}Container already exists: $CONTAINER${NC}"
      echo "  Set HIVY_EVAL_CONTAINER to a new name or EVAL_REPLACE_CONTAINER=1 to replace it."
      exit 1
    fi
  fi

  docker run -d --name "$CONTAINER" \
    -p "${PORT}:7080" \
    -e "HIVY_RUNTIME_SECRET=$SECRET" \
    -e "HIVY_PROXY_API_KEY=$API_KEY" \
    -e "HIVY_GIT_USERNAME=$GIT_USERNAME" \
    -e "HIVY_GIT_EMAIL=$GIT_EMAIL" \
    -e "RUST_LOG=info" \
    "$IMAGE" >/dev/null

  docker logs -f "$CONTAINER" >"$EVAL_DOCKER_LOG" 2>&1 &
  DOCKER_LOG_PID="$!"

  echo "  Container:  $CONTAINER"
  echo "  URL:        $BASE_URL"
  echo "  Docker log: $EVAL_DOCKER_LOG"

  if ! wait_for_runtime_path "healthz" "healthz"; then
    docker logs "$CONTAINER" | tail -20
    exit 1
  fi
}

create_rails_app() {
  echo -e "${BLUE}[3/7] Creating Rails ${RAILS_VERSION} app in ${APP_PATH}...${NC}"

  docker exec "$CONTAINER" bash -c "
    set -euo pipefail
    export HOME=/workspace
    hash -r
    cd /workspace
    if ! ruby -e 'gem \"rails\", \"${RAILS_VERSION}\"' >/dev/null 2>&1; then
      gem install --no-document rails -v '${RAILS_VERSION}'
    fi
    if [[ ! -d app ]]; then
      rails _${RAILS_VERSION}_ new app --database=sqlite3 --skip-jbuilder
    fi
    cd app
    bundle install
    bin/rails db:prepare
    printf 'ruby=%s\n' \"\$(ruby -v)\"
    printf 'rails=%s\n' \"\$(bin/rails --version)\"
    bin/rails runner 'puts :rails_runner_ok'
  "
}

verify_runtime_image() {
  local image_id

  image_id=$(docker image inspect "$IMAGE" --format '{{.Id}}')
  echo "  Image ID:   $image_id"

  if ! docker run --rm --entrypoint /bin/bash "$IMAGE" -c \
    "grep -a -q 'database event queue' /usr/local/bin/hivy-sandboxes-runtime"; then
    echo -e "  ${RED}Built image does not contain the database event queue runtime strings.${NC}"
    echo "  Refusing to continue because this usually means a stale binary was packaged."
    exit 1
  fi

  echo -e "  ${GREEN}Runtime image sanity check passed: database queue code is present.${NC}"
}
