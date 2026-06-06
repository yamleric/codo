# Build stage
FROM golang:1.26-alpine AS builder
ENV PLAYWRIGHT_DRIVER_PATH=/ms-playwright-go \
    PLAYWRIGHT_NODEJS_PATH=/usr/bin/node
WORKDIR /app
RUN apk add --no-cache nodejs
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api && \
    CGO_ENABLED=0 go build -o /scheduler ./cmd/scheduler && \
    go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1 --version

# Frontend build
FROM node:22-alpine AS frontend
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Final image
FROM alpine:3.21
ARG YTDLP_VERSION=2026.03.17
ENV PLAYWRIGHT_DRIVER_PATH=/ms-playwright-go \
    PLAYWRIGHT_NODEJS_PATH=/usr/bin/node \
    PLAYWRIGHT_CHROMIUM_EXECUTABLE=/usr/bin/chromium-browser
RUN apk add --no-cache ca-certificates tzdata ffmpeg python3 py3-pip nodejs chromium && \
    pip3 install --no-cache-dir --break-system-packages "yt-dlp[default]==${YTDLP_VERSION}"
WORKDIR /app
COPY --from=builder /api .
COPY --from=builder /scheduler .
COPY --from=builder /ms-playwright-go /ms-playwright-go
COPY --from=frontend /web/dist ./web/dist
EXPOSE 8080
CMD ["./api"]
