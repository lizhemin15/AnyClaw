FROM node:20-alpine AS web
WORKDIR /app
COPY AnyClaw-Web/package.json AnyClaw-Web/package-lock.json* ./
RUN npm install
COPY AnyClaw-Web/ .
ARG CACHE_BUST
RUN echo "Build: ${CACHE_BUST:-dev}"
ENV VITE_API_URL=
RUN npm run build

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY AnyClaw-API/go.mod AnyClaw-API/go.sum ./
RUN go mod download
COPY AnyClaw-API/ ./
COPY --from=web /app/dist ./internal/web/dist
ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X github.com/anyclaw/anyclaw-api/internal/buildinfo.GitCommit=${GIT_SHA} -X github.com/anyclaw/anyclaw-api/internal/buildinfo.BuildTime=${BUILD_TIME}" \
  -o /anyclaw-api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
COPY --from=builder /anyclaw-api /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/anyclaw-api"]
