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

RUN apk add --no-cache ca-certificates tzdata curl bash
ENV TZ=Asia/Shanghai

# 预装 Skillhub CLI（腾讯云镜像，国内加速、合规）
RUN curl -fsSL https://skillhub-1388575217.cos.ap-guangzhou.myqcloud.com/install/install.sh | bash -s -- --cli-only || true

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

# Copy binary
COPY --from=builder /src/build/anyclaw /usr/local/bin/anyclaw

COPY config/config.example.json /opt/anyclaw/config.example.json

# Create non-root user and group
RUN addgroup -g 1000 anyclaw && \
    adduser -D -u 1000 -G anyclaw anyclaw

# Switch to non-root user
USER anyclaw

# Run onboard to create initial directories and config
RUN /usr/local/bin/anyclaw onboard

ENTRYPOINT ["anyclaw"]
CMD ["gateway"]
