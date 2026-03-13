# ============================================================
# Stage 1: Build the anyclaw binary
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN make build

# ============================================================
# Stage 2: Minimal runtime image
# ============================================================
FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata curl
ENV TZ=Asia/Shanghai

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

# Copy binary
COPY --from=builder /src/build/anyclaw /usr/local/bin/anyclaw

# 预制 Skillhub 商店配置（参考用，onboard 会从 DefaultConfig 生成含 Skillhub 的 config）
COPY config/config.example.json /opt/anyclaw/config.example.json

# Create non-root user and group
RUN addgroup -g 1000 anyclaw && \
    adduser -D -u 1000 -G anyclaw anyclaw

# Switch to non-root user
USER anyclaw

# Run onboard to create initial directories and config (Skillhub 已内置在 DefaultConfig)
RUN /usr/local/bin/anyclaw onboard

ENTRYPOINT ["anyclaw"]
CMD ["gateway"]
