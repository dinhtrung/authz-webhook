# ===========================================
# Build Stage
# ===========================================
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum (if they exist) and vendor directory
COPY go.mod go.sum ./
COPY vendor ./vendor

# Copy the main application file
COPY main.go .

# Build the static binary
# -mod=vendor tells Go to use the local vendor directory
# CGO_ENABLED=0 ensures a fully static binary for Alpine/distroless
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o /tinyauth .

# ===========================================
# Runtime Stage
# ===========================================
FROM alpine:3.21

# Install CA certificates for HTTPS connections (needed for Kratos API)
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /tinyauth .

# Change ownership to the non-root user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port 5000 (default for the app)
EXPOSE 5000

# Set default arguments
# You can override these at runtime with: docker run <image> --kratos-admin-url=http://new-url:port/admin/identities
ENTRYPOINT ["/app/tinyauth"]
CMD ["--addr", ":5000"]
