# Ambiente per compilare Leggio per Linux. Ubuntu 22.04 perché la sua glibc
# è abbastanza vecchia: l'eseguibile parte anche sulle distribuzioni del 2022.
ARG BASE=amd64/ubuntu:22.04
FROM ${BASE}
ARG GO_VERSION=1.26.4
ARG ARCH=amd64
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      build-essential pkg-config ca-certificates curl \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH GOFLAGS=-buildvcs=false
WORKDIR /src
