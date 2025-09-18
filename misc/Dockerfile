FROM debian:bookworm-slim AS base

# Local build:
#     ./misc/utils.sh build-target x86_64
#     ./misc/utils.sh download-debian-libusb
#     docker build -t test -f ./misc/Dockerfile .

# amd64-specific stage
FROM base AS build-amd64
COPY build/x86_64 /app/
COPY libusb/x86_64-linux-gnu/libusb-1.0.so.0 /usr/lib/x86_64-linux-gnu/

# arm64-specific stage
FROM base AS build-arm64
COPY build/aarch64 /app/
COPY libusb/aarch64-linux-gnu/libusb-1.0.so.0 /usr/lib/aarch64-linux-gnu/

# common steps
FROM build-${TARGETARCH} AS build
WORKDIR /app
ENTRYPOINT ["/app/sentryshot", "run", "--config", "/app/configs/sentryshot.toml"]
