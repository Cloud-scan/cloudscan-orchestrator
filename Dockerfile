# Dockerfile for CloudScan Orchestrator
# Expects pre-built binary from make linux
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 cloudscan && \
    adduser -D -u 1000 -G cloudscan cloudscan

WORKDIR /app

# Copy pre-built binary (expects cloudscan-orchestrator-amd64 or cloudscan-orchestrator-arm64)
ARG TARGETARCH
COPY cloudscan-orchestrator-${TARGETARCH} ./cloudscan-orchestrator

# Create necessary directories with proper permissions
RUN mkdir -p /app/cache /tmp && \
    chown -R cloudscan:cloudscan /app /tmp

# Switch to non-root user
USER cloudscan

# Expose ports (gRPC and HTTP)
EXPOSE 9999 8081

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Run the binary
ENTRYPOINT ["/app/cloudscan-orchestrator"]