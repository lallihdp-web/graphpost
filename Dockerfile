# Build stage
FROM golang:1.21-alpine AS builder

# Build argument to enable CGO (for DuckDB support)
ARG CGO_ENABLED=0

WORKDIR /app

# Install dependencies
# Add build-base for CGO builds
RUN apk add --no-cache git ca-certificates && \
    if [ "$CGO_ENABLED" = "1" ]; then apk add --no-cache build-base; fi

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
# CGO_ENABLED=0: Lightweight build without DuckDB analytics
# CGO_ENABLED=1: Full build with DuckDB analytics (requires larger image)
RUN CGO_ENABLED=${CGO_ENABLED} GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o graphpost ./cmd/graphpost

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/graphpost .

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run
ENTRYPOINT ["./graphpost"]
