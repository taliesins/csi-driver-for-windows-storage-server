ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/go:1-1.24-bookworm

# hadolint ignore=DL3006
FROM $BASE_IMAGE

# Set the shell to fail on errors and pipe failures
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Install Python
ARG PYTHON_VERSION=3.11.2-1+b1
ARG PYTHON_VENV_VERSION=3.11.2-1+b1
ARG PIP_VERSION=23.0.1+dfsg-1
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      python3=${PYTHON_VERSION} \
      python3-venv=${PYTHON_VENV_VERSION} \
      python3-pip=${PIP_VERSION} \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Node
ARG NODE_VERSION=22
ARG NODEJS_VERSION="22.10.0-1nodesource1"
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - && \
    apt-get install -y --no-install-recommends nodejs=${NODEJS_VERSION} && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

#Get Docker CLI
ARG DOCKER_CLI_VERSION="5:28.2.2-1~debian.12~bookworm"
ARG DOCKER_BUILDX_VERSION="0.24.0-1~debian.12~bookworm"
ARG CONTAINERD_VERSION="1.7.27-1"
ARG DOCKER_COMPOSE_PLUGIN="2.36.2-1~debian.12~bookworm"
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc && \
    echo \
    "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian \
    bookworm stable" > /etc/apt/sources.list.d/docker.list && \
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
ENV PYTHONPATH="/usr/local/lib/python3.11/site-packages${PYTHONPATH:+:$PYTHONPATH}"

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
ARG TRIVY_VERSION=0.63.0
RUN curl -sSL https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz -o trivy.tar.gz \
    && tar -xf trivy.tar.gz \
    && mv trivy /usr/bin/ \
    && mv contrib /home/vscode \
    && chown -R vscode:vscode /home/vscode/contrib \
    && chmod +x /usr/bin/trivy \
    && rm -f trivy.tar.gz \
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
RUN mkdir -p /home/${CONTAINER_USER}/.aws /home/${CONTAINER_USER}/.azure /home/${CONTAINER_USER}/.kube /home/${CONTAINER_USER}/.packer.d /home/${CONTAINER_USER}/.terraform.d /home/${CONTAINER_USER}/.ssh /workspace/node_modules /workspace/.next /workspace/cache \
    && chown -R ${CONTAINER_USER}:${CONTAINER_GROUP} /home/${CONTAINER_USER} /workspace \
    && SNIPPET="export PROMPT_COMMAND='history -a' && export HISTFILE=/home/${CONTAINER_USER}/.bash_history" \
    && echo "$SNIPPET" >> "/home/${CONTAINER_USER}/.bashrc" \
    && chmod 700 /home/${CONTAINER_USER}/.ssh \
    && find /home/${CONTAINER_USER}/.ssh -type f -exec chmod 600 {} +

USER ${CONTAINER_USER}

# goimports
ARG GOIMPORTS_VERSION=v0.20.0
RUN go install golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}

CMD ["sleep", "infinity"]
