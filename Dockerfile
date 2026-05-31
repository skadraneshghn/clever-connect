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

# Copy codebase
COPY . .

# Compile Client SPA Frontend
RUN cd web/client && bun install && bun run build

# Compile Server SPA Frontend
RUN cd web/server && bun install && bun run build

# Compile Go backend binary with embedded distributions
RUN go build -o bin/clever-connect main.go

# Expose ports
EXPOSE 8080
EXPOSE 8081

# Default environment configuration (Clever Cloud will override these)
ENV APP_MODE=server
ENV PORT=8080

CMD ["./bin/clever-connect"]
