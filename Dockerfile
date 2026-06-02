# Use the latest Ubuntu image as base
FROM ubuntu:latest

# Avoid interactive prompts during apt-get package installs
ENV DEBIAN_FRONTEND=noninteractive

# Update system and install essential tools
RUN apt-get update && apt-get install -y \
    curl \
    git \
    build-essential \
    ca-certificates \
    unzip \
    nginx \
    && rm -rf /var/lib/apt/lists/*

# Install stable Go 1.22.3
RUN curl -OL https://go.dev/dl/go1.22.3.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz && \
    rm go1.22.3.linux-amd64.tar.gz

ENV PATH=$PATH:/usr/local/go/bin

# Install Node.js v20 (LTS)
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs

# Install Bun
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH=$PATH:/root/.bun/bin

# Set workspace directory
WORKDIR /app

# ==========================================
# PHASE 1: CACHE GO DEPENDENCIES
# ==========================================
# Copy go.mod and go.sum first to cache dependency download layer
COPY go.mod go.sum ./
RUN go mod download

# ==========================================
# PHASE 2: CACHE SERVER NODE DEPENDENCIES
# ==========================================
# Copy package.json / bun.lock for server
COPY web/server/package.json web/server/bun.lock ./web/server/
RUN cd web/server && bun install

# ==========================================
# PHASE 3: COPY CODEBASE & COMPILE
# ==========================================
# Now copy the actual source code
COPY . .

# Create empty placeholder for client dist to satisfy go:embed when compiling the binary
RUN mkdir -p web/client/dist && touch web/client/dist/index.html

# Compile Server SPA Frontend
RUN cd web/server && bun run build

# Compile Go backend binary with embedded distributions
RUN go build -o bin/clever-connect main.go

# Resolve transitive dependencies of Ehco to populate go.sum dynamically
RUN go get github.com/Ehco1996/ehco/cmd/ehco

# Compile the Ehco binary so it's baked into the image
RUN go build -o bin/ehco github.com/Ehco1996/ehco/cmd/ehco

# Create the data directory for dynamic JSON configs and ensure permissions
RUN mkdir -p data && chmod 777 data

# Copy Nginx config
COPY nginx.conf /etc/nginx/nginx.conf

# Default environment configuration (Clever Cloud will override these)
ENV APP_MODE=server
ENV PORT=8080

# Start Nginx in background, set Gin port to 3000, and run Go backend directly as main process (PID 1)
CMD service nginx start && export PORT=3000 && exec ./bin/clever-connect
