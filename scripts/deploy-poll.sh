#!/bin/bash
set -euo pipefail

# Polling-based deployment script for helloworld-ai
# This script checks for new Docker images and updates containers if a newer version is available
# Designed to be run periodically via cron (e.g., every 5-15 minutes)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

# Configuration
# Set GITHUB_REPOSITORY environment variable on your production server
# Example: export GITHUB_REPOSITORY="your-username/helloworld-ai"
IMAGE_NAME="ghcr.io/${GITHUB_REPOSITORY:-your-username/helloworld-ai}:latest"
COMPOSE_FILE="docker-compose.yml"
POLL_INTERVAL=${POLL_INTERVAL:-300}  # Default: 5 minutes (300 seconds)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

log_debug() {
    echo -e "${BLUE}[DEBUG]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null && ! command -v docker &> /dev/null; then
    log_error "Docker is not installed or not in PATH"
    exit 1
fi

# Use docker compose (v2) if available, otherwise docker-compose (v1)
if docker compose version &> /dev/null; then
    COMPOSE_CMD="docker compose"
else
    COMPOSE_CMD="docker-compose"
fi

# Get current image digest (if container is running)
get_current_digest() {
    if docker ps --format '{{.Names}}' | grep -q "^helloworld-ai-api$"; then
        docker inspect --format='{{index .RepoDigests 0}}' helloworld-ai-api 2>/dev/null | cut -d'@' -f2 || echo ""
    else
        echo ""
    fi
}

# Get latest image digest from registry
get_latest_digest() {
    # Try to pull image manifest to get digest
    # Note: This requires authentication if the repository is private
    docker manifest inspect "${IMAGE_NAME}" 2>/dev/null | \
        grep -o '"digest":"[^"]*"' | head -1 | cut -d'"' -f4 || echo ""
}

# Check if new image is available
check_for_updates() {
    log_info "Checking for updates..."
    
    # Pull latest image metadata (doesn't download full image)
    log_debug "Fetching latest image metadata..."
    if ! docker pull "${IMAGE_NAME}" > /dev/null 2>&1; then
        log_warn "Failed to pull image metadata (may need authentication or network issue)"
        return 1
    fi
    
    local current_digest=$(get_current_digest)
    local latest_digest=$(docker inspect --format='{{index .RepoDigests 0}}' "${IMAGE_NAME}" 2>/dev/null | cut -d'@' -f2 || echo "")
    
    if [ -z "$latest_digest" ]; then
        log_warn "Could not determine latest image digest"
        return 1
    fi
    
    if [ -z "$current_digest" ]; then
        log_info "No running container found, new image available"
        return 0
    fi
    
    if [ "$current_digest" != "$latest_digest" ]; then
        log_info "New image available (current: ${current_digest:0:12}..., latest: ${latest_digest:0:12}...)"
        return 0
    else
        log_debug "Already running latest version (${latest_digest:0:12}...)"
        return 1
    fi
}

# Perform deployment
deploy() {
    log_info "Starting deployment of ${IMAGE_NAME}"
    
    # Pull latest image
    log_info "Pulling latest Docker image..."
    if ! docker pull "${IMAGE_NAME}"; then
        log_error "Failed to pull Docker image"
        return 1
    fi
    
    # Check if containers are running
    if ${COMPOSE_CMD} -f "${COMPOSE_FILE}" ps | grep -q "helloworld-ai-api.*Up"; then
        log_info "Containers are running, performing rolling update..."
        
        # Start new containers with updated image
        ${COMPOSE_CMD} -f "${COMPOSE_FILE}" up -d --no-deps api || {
            log_error "Failed to update API container"
            return 1
        }
        
        # Wait for health check
        log_info "Waiting for API to be healthy..."
        max_attempts=30
        attempt=0
        while [ $attempt -lt $max_attempts ]; do
            if docker inspect --format='{{.State.Health.Status}}' helloworld-ai-api 2>/dev/null | grep -q "healthy"; then
                log_info "API is healthy"
                break
            fi
            attempt=$((attempt + 1))
            sleep 2
        done
        
        if [ $attempt -eq $max_attempts ]; then
            log_warn "API health check timeout, but container is running"
        fi
        
        # Remove old containers
        ${COMPOSE_CMD} -f "${COMPOSE_FILE}" rm -f || true
    else
        log_info "Containers are not running, starting fresh..."
        ${COMPOSE_CMD} -f "${COMPOSE_FILE}" up -d || {
            log_error "Failed to start containers"
            return 1
        }
    fi
    
    # Show container status
    log_info "Container status:"
    ${COMPOSE_CMD} -f "${COMPOSE_FILE}" ps
    
    log_info "Deployment completed successfully!"
    return 0
}

# Main polling loop
poll_loop() {
    log_info "Starting polling loop (checking every ${POLL_INTERVAL} seconds)"
    log_info "Press Ctrl+C to stop"
    
    while true; do
        if check_for_updates; then
            deploy
        fi
        
        log_debug "Sleeping for ${POLL_INTERVAL} seconds..."
        sleep "${POLL_INTERVAL}"
    done
}

# Main execution
main() {
    # Check if running in polling mode or one-shot mode
    if [ "${1:-}" = "--poll" ]; then
        poll_loop
    else
        # One-shot check and deploy
        if check_for_updates; then
            deploy
        else
            log_info "Already running latest version, no deployment needed"
        fi
    fi
}

main "$@"

