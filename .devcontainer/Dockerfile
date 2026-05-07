ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/go:1.26-trixie

# hadolint ignore=DL3006
FROM $BASE_IMAGE

# Set the shell to fail on errors and pipe failures
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Refresh the Yarn apt key inherited from the base image. The key bundled in
# older devcontainer images can expire before the image itself is refreshed.
RUN curl -fsSL https://dl.yarnpkg.com/debian/pubkey.gpg \
    | gpg --batch --yes --dearmor -o /usr/share/keyrings/yarn-archive-keyring.gpg

# Install Python
ARG PYTHON_VERSION=3.13.5-1
ARG PYTHON_VENV_VERSION=3.13.5-1
ARG PIP_VERSION=25.1.1+dfsg-1
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      python3=${PYTHON_VERSION} \
      python3-venv=${PYTHON_VENV_VERSION} \
      python3-pip=${PIP_VERSION} \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Node
ARG NODE_VERSION=25
ARG NODEJS_VERSION="25.9.0-1nodesource1"
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - && \
    apt-get install -y --no-install-recommends nodejs=${NODEJS_VERSION} && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install npm and Bun
ARG NPM_VERSION=11.13.0
ARG BUN_VERSION=1.3.13
ARG BUN_TARGET=bun-linux-x64
RUN mkdir -p /usr/local/lib/node_modules/npm \
    && curl -fsSL "https://registry.npmjs.org/npm/-/npm-${NPM_VERSION}.tgz" -o /tmp/npm.tgz \
    && tar xzf /tmp/npm.tgz -C /usr/local/lib/node_modules/npm --strip-components=1 \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && curl -fsSL "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/${BUN_TARGET}.zip" -o /tmp/bun.zip \
    && unzip -q /tmp/bun.zip -d /tmp \
    && install -m 0755 "/tmp/${BUN_TARGET}/bun" /usr/local/bin/bun \
    && rm -rf /tmp/npm.tgz /tmp/bun.zip "/tmp/${BUN_TARGET}" \
    && node --version \
    && npm --version \
    && bun --version

#Get Docker CLI
ARG DEBIAN_CODENAME=trixie
ARG DOCKER_CLI_VERSION="5:29.4.1-1~debian.13~trixie"
ARG DOCKER_BUILDX_VERSION="0.33.0-1~debian.13~trixie"
ARG CONTAINERD_VERSION="2.2.3-1~debian.13~trixie"
ARG DOCKER_COMPOSE_PLUGIN="5.1.3-1~debian.13~trixie"
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc && \
    echo \
    "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian \
    ${DEBIAN_CODENAME} stable" > /etc/apt/sources.list.d/docker.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
      docker-ce-cli=${DOCKER_CLI_VERSION} \
      docker-buildx-plugin=${DOCKER_BUILDX_VERSION} \
      containerd.io=${CONTAINERD_VERSION} \
      docker-compose-plugin=${DOCKER_COMPOSE_PLUGIN} \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Get uv
ARG UV_VERSION=0.7.17
RUN curl -sSL https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-x86_64-unknown-linux-gnu.tar.gz -o uv.tar.gz \
    && tar xzf uv.tar.gz \
    && mv uv-x86_64-unknown-linux-gnu/uv /usr/bin/ \
    && mv uv-x86_64-unknown-linux-gnu/uvx /usr/bin/ \
    && chmod +x /usr/bin/uv /usr/bin/uvx \
    && rm -rf uv.tar.gz uv-x86_64-unknown-linux-gnu

# Export site-packages to PYTHONPATH explicitly
ENV PATH="/usr/local/bin:$PATH"
ENV PYTHONPATH="/usr/local/lib/python3.13/site-packages"

# Get pre-commit
ARG PRECOMMIT_VERSION=4.2.0
RUN uv pip install --no-cache-dir --prefix=/usr/local pre-commit==${PRECOMMIT_VERSION}

# Get helm
ARG HELM_VERSION=3.18.3
RUN curl -sSL https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz -o helm.tgz \
    && tar xzf helm.tgz \
    && mv linux-amd64/helm /usr/bin/ \
    && chmod +x /usr/bin/helm \
    && rm -rf helm.tgz linux-amd64

# Get helm-docs
ARG HELM_DOCS_VERSION=v1.14.2
RUN GOBIN=/usr/local/bin go install github.com/norwoodj/helm-docs/cmd/helm-docs@${HELM_DOCS_VERSION}

# Get shfmt
ARG SHFMT_VERSION=3.11.0
RUN curl -sSL https://github.com/mvdan/sh/releases/download/v${SHFMT_VERSION}/shfmt_v${SHFMT_VERSION}_linux_amd64 -o /usr/bin/shfmt \
    && chmod +x /usr/bin/shfmt

# Get shellcheck
ARG SHELLCHECK_VERSION=0.10.0
RUN curl -sSL https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.xz -o shellcheck.tar.xz \
    && tar xf shellcheck.tar.xz \
    && mv shellcheck-v${SHELLCHECK_VERSION}/shellcheck /usr/bin/ \
    && chmod +x /usr/bin/shellcheck \
    && rm -rf shellcheck.tar.xz shellcheck-v${SHELLCHECK_VERSION}

# Get hadolint
ARG HADOLINT_VERSION=2.12.0
RUN curl -sSL https://github.com/hadolint/hadolint/releases/download/v${HADOLINT_VERSION}/hadolint-Linux-x86_64 -o /usr/bin/hadolint \
    && chmod +x /usr/bin/hadolint

# Get trivy
ARG TRIVY_VERSION=0.70.0
RUN curl -fsSL https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz -o trivy.tar.gz \
    && tar xzf trivy.tar.gz \
    && mv trivy /usr/bin/ \
    && if [ -d contrib ]; then mv contrib /home/vscode && chown -R vscode:vscode /home/vscode/contrib; fi \
    && chmod +x /usr/bin/trivy \
    && rm -f trivy.tar.gz LICENSE README.md \
    && su vscode -c "trivy image --download-db-only --no-progress"

# golangci-lint
ARG GOLANGCILINT_VERSION=v2.2.0
RUN curl -sSL https://github.com/golangci/golangci-lint/releases/download/${GOLANGCILINT_VERSION}/golangci-lint-${GOLANGCILINT_VERSION#v}-linux-amd64.tar.gz -o golangci-lint.tar.gz \
    && tar xzf golangci-lint.tar.gz \
    && mv golangci-lint-${GOLANGCILINT_VERSION#v}-linux-amd64/golangci-lint /usr/bin/ \
    && chmod +x /usr/bin/golangci-lint \
    && rm -rf golangci-lint.tar.gz golangci-lint-${GOLANGCILINT_VERSION#v}-linux-amd64

# goreleaser
ARG GORELEASER_VERSION=v2.10.2
RUN curl -sSL https://github.com/goreleaser/goreleaser/releases/download/${GORELEASER_VERSION}/goreleaser_Linux_x86_64.tar.gz -o goreleaser.tar.gz \
    && tar xzf goreleaser.tar.gz \
    && mv goreleaser /usr/bin/ \
    && chmod +x /usr/bin/goreleaser \
    && rm -f goreleaser.tar.gz LICENSE.md README.md

# Create and set permissions for various tooling directories
ARG CONTAINER_USER=vscode
ARG CONTAINER_GROUP=vscode
USER root
# When sharing files between devcontainers we need to have the same guid between them. The default GID is 1000 for vscode.
RUN set -eux; \
    # Ensure group exists for the GID
    if ! getent group "${CONTAINER_GROUP}" >/dev/null; then \
    groupadd --gid "${CONTAINER_GROUP}" shared-group; \
    fi; \
    # Resolve group name from GID
    GROUP_NAME="$(getent group "${CONTAINER_GROUP}" | cut -d: -f1)"; \
    # Only add user to group if not already a member
    if ! id -nG "${CONTAINER_USER}" | grep -qw "${GROUP_NAME}"; then \
    usermod -aG "${GROUP_NAME}" "${CONTAINER_USER}"; \
    fi

RUN groupadd -g 1001 docker \
    && usermod -aG docker root \
    && usermod -aG docker ${CONTAINER_USER}

RUN mkdir -p /home/${CONTAINER_USER}/.cache/go-build /home/${CONTAINER_USER}/.kube /home/${CONTAINER_USER}/.ssh /go/bin /go/pkg/mod /workspace \
    && chown -R ${CONTAINER_USER}:${CONTAINER_GROUP} /home/${CONTAINER_USER} /go /workspace \
    && SNIPPET="export PROMPT_COMMAND='history -a' && export HISTFILE=/home/${CONTAINER_USER}/.bash_history" \
    && echo "$SNIPPET" >> "/home/${CONTAINER_USER}/.bashrc" \
    && chmod 700 /home/${CONTAINER_USER}/.ssh \
    && find /home/${CONTAINER_USER}/.ssh -type f -exec chmod 600 {} +

USER ${CONTAINER_USER}

# goimports
ARG GOIMPORTS_VERSION=v0.44.0
RUN go install golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}

CMD ["sleep", "infinity"]
