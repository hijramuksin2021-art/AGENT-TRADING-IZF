# IZF Trading Platform - Build from Source
# Backend: Go + TA-Lib | Frontend: Node/React | Runtime: Alpine + Nginx

# ─── Stage 1: Build TA-Lib ───────────────────────────────────────
FROM alpine:latest AS ta-lib-builder

RUN apk update && apk add --no-cache wget tar make gcc g++ musl-dev autoconf automake

RUN wget http://prdownloads.sourceforge.net/ta-lib/ta-lib-0.4.0-src.tar.gz && \
    tar -xzf ta-lib-0.4.0-src.tar.gz && \
    cd ta-lib && \
    if [ "$(uname -m)" = "aarch64" ]; then \
        CONFIG_GUESS=$(find /usr/share -name config.guess | head -1) && \
        CONFIG_SUB=$(find /usr/share -name config.sub | head -1) && \
        cp "$CONFIG_GUESS" config.guess && \
        cp "$CONFIG_SUB" config.sub && \
        chmod +x config.guess config.sub; \
    fi && \
    ./configure --prefix=/usr/local && \
    make && make install && \
    cd .. && rm -rf ta-lib ta-lib-0.4.0-src.tar.gz

# ─── Stage 2: Build Go Backend ───────────────────────────────────
FROM golang:1.25-alpine AS backend-builder

RUN apk update && apk add --no-cache git make gcc g++ musl-dev

COPY --from=ta-lib-builder /usr/local /usr/local

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux \
    CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
    go build -trimpath -ldflags="-s -w" -o izf-server .

# ─── Stage 3: Build React Frontend ───────────────────────────────
FROM node:20-alpine AS frontend-builder

WORKDIR /build
COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ─── Stage 4: Runtime (Alpine + Nginx) ───────────────────────────
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata sqlite nginx openssl

COPY --from=ta-lib-builder /usr/local /usr/local
RUN ldconfig /usr/local/lib 2>/dev/null || true

WORKDIR /app
RUN mkdir -p /app/data /run/nginx

COPY --from=backend-builder /app/izf-server /app/izf-server
COPY --from=frontend-builder /build/dist /usr/share/nginx/html
COPY railway/start.sh /app/start.sh
RUN chmod +x /app/start.sh && \
    sed -i 's|/app/nofx|/app/izf-server|g' /app/start.sh

ENV DB_PATH=/app/data/data.db
ENV TZ=Asia/Jakarta

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

CMD ["/app/start.sh"]
