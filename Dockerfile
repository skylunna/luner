# 阶段 1: 编译（Go 1.24）
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /luner ./cmd/luner

# 阶段 2: 运行（最小化运行时）
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata curl
WORKDIR /app
COPY --from=builder /luner .
COPY config/config.example.yaml ./config.yaml
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/luner"]
CMD ["-config", "config.yaml"]