# Stage 1: Build web console
FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci --silent
COPY web/ ./
# Vite outputs to ../internal/console/dist relative to web/ → /app/internal/console/dist
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

# Cache dependency layer separately
COPY go.mod go.sum ./
RUN go mod download

# Copy source, then overlay the freshly built frontend
COPY . .
COPY --from=frontend-builder /app/internal/console/dist ./internal/console/dist

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/skylunna/luner/cmd/luner.Version=docker" \
    -o /luner \
    ./cmd/luner

# Stage 3: Minimal runtime image
FROM alpine:3.24
RUN apk --no-cache add ca-certificates tzdata curl

# Non-root user
RUN addgroup -g 1000 luner && \
    adduser -D -u 1000 -G luner luner

# Directories owned by the luner user
RUN mkdir -p /data /etc/luner && \
    chown luner:luner /data /etc/luner

COPY --from=go-builder /luner /usr/local/bin/luner

USER luner
WORKDIR /data

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -sf http://localhost:8080/api/health || exit 1

ENTRYPOINT ["luner"]
CMD ["--config", "/etc/luner/config.yaml"]
