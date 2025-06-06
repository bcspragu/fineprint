# docker build -t registryext.bsprague.com/fineprint .

# Build stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go main.go
COPY claude/ claude/
COPY diff/ diff/
COPY htmlutil/ htmlutil/
COPY postmark/ postmark/
COPY ratelimit/ ratelimit/
COPY templates/ templates/
COPY tosdr/ tosdr/
COPY webarchive/ webarchive/

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o fineprint .

# Runtime stage, use latest LTS Node image
FROM node:22-alpine

# Install dependencies for Node.js runtime
RUN apk add --no-cache ca-certificates

# Create app directory
WORKDIR /app

# Copy package.json and install Node dependencies
COPY mjml/package.json ./
RUN npm install --omit=dev

# Copy the compiled Go binary
COPY --from=builder /app/fineprint .

# Copy the Node.js script
COPY mjml/compile-mjml.js ./mjml/compile-mjml.js

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -S appuser -u 1001 -G appgroup

# Change ownership
RUN chown -R appuser:appgroup /app
USER appuser

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/fineprint"]
