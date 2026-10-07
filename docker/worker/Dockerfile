# Stage 1: Build binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Enable automatic toolchain download for Go 1.26.4
ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source files
COPY . .

# Build Worker binary
RUN go build -ldflags="-s -w" -o /bin/worker ./cmd/worker

# Stage 2: Runtime image
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/worker /app/worker

ENTRYPOINT ["/app/worker"]
