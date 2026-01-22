FROM mcr.microsoft.com/devcontainers/go:1

# Install OS packages needed for development and MeCab
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        git \
        tzdata \
        mecab \
        libmecab-dev \
        mecab-ipadic-utf8 \
        sqlite3 \
        libsqlite3-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Install golangci-lint
ARG GOLANGCI_LINT_VERSION=v1.59.0
RUN curl -sSfL https://github.com/golangci/golangci-lint/releases/download/${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION#v}-linux-amd64.tar.gz \
    | tar -xz -C /tmp \
    && mv /tmp/golangci-lint-${GOLANGCI_LINT_VERSION#v}-linux-amd64/golangci-lint /usr/local/bin/ \
    && chmod +x /usr/local/bin/golangci-lint

# Install delve for debugging
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Ensure $GOPATH/bin is in PATH
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

# Create workspace dir
WORKDIR /workspace
