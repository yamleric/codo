# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api && \
    CGO_ENABLED=0 go build -o /scheduler ./cmd/scheduler

# Frontend build
FROM node:22-alpine AS frontend
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Final image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /api .
COPY --from=builder /scheduler .
COPY --from=frontend /web/dist ./web/dist
EXPOSE 8080
CMD ["./api"]
