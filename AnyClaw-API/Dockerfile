FROM node:20-alpine AS web
WORKDIR /app
COPY AnyClaw-Web/package.json AnyClaw-Web/package-lock.json* ./
RUN npm install
COPY AnyClaw-Web/ .
ENV VITE_API_URL=
RUN npm run build

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY AnyClaw-API/go.mod AnyClaw-API/go.sum ./
RUN go mod download
COPY AnyClaw-API/ ./
COPY --from=web /app/dist ./internal/web/dist
RUN go mod tidy && CGO_ENABLED=0 go build -o /anyclaw-api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /anyclaw-api /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/anyclaw-api"]
