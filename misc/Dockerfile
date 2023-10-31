FROM debian:bookworm-slim AS base

# amd64-specific stage
FROM base AS build-amd64
COPY build/x86_64 /app/

# arm64-specific stage
FROM base AS build-arm64
COPY build/aarch64 /app/

# common steps
FROM build-${TARGETARCH} AS build
WORKDIR /app
ENTRYPOINT /app/sentryshot run --config /app/configs/sentryshot.toml
