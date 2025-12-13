# Deployment Guide

This guide explains how to deploy helloworld-ai to your production server.

## Development vs Production

- **Local Development**: Use Tilt (`make start` or `tilt up`) - manages all services including llama.cpp
- **Production**: Use Docker Compose for API + Qdrant, run llama.cpp separately

## Overview

The deployment pipeline works as follows:

1. **GitHub Actions** builds and tests the code on every push to `main`
2. **Docker image** is built and pushed to GitHub Container Registry (GHCR)
3. **Polling script** on your production server periodically checks for new images
4. **Deployment script** pulls the new image and updates containers gracefully

**Note:** This repository only builds the API backend. You'll need to handle Qdrant (via Docker Compose) and llama.cpp (separately) on your production server.

## Prerequisites

### On Your Production Server

1. **Docker and Docker Compose** installed
2. **llama.cpp** built and running separately (not in Docker)
3. **Project directory** cloned on your production server
4. **Environment variables** configured
5. **GitHub Container Registry access** (for pulling images)

## Production Setup

### 1. Build and Run llama.cpp Server

The llama.cpp server must be built and run separately on your production server. It should NOT be run as a Docker container.

```bash
# Build llama.cpp (example - adjust to your setup)
cd /path/to/llama.cpp
make

# Run llama.cpp server
./build/bin/llama-server \
  --models-dir /path/to/models \
  --port 8081 \
  --host 0.0.0.0 \
  --embeddings
```

Ensure the llama.cpp server is:
- Accessible at the URL you'll configure in `LLM_BASE_URL`
- Running on a port that's accessible from Docker containers (use `host.docker.internal:8081` if running on host)
- Configured with the models you need (chat and embeddings)

### 2. Prepare Production Server Environment

On your production server:

```bash
# Clone repository
git clone <your-repo-url> ~/helloworld-ai
cd ~/helloworld-ai

# Set GitHub repository name (replace with your actual repo)
export GITHUB_REPOSITORY="your-username/helloworld-ai"

# Create .env file
cp .env.example .env
nano .env
```

### 3. Configure Environment Variables

Create a `.env` file with the following variables:

```bash
# LLM Configuration (llama.cpp server running separately on host)
LLM_BASE_URL=http://host.docker.internal:8081
LLM_MODEL=Qwen2.5-3B-Instruct-Q4_K_M
LLM_API_KEY=dummy-key

# Embeddings Configuration
EMBEDDING_BASE_URL=http://host.docker.internal:8081
EMBEDDING_MODEL_NAME=ggml-org_embeddinggemma-300M-GGUF_embeddinggemma-300M-Q8_0

# Database
DB_PATH=/app/data/helloworld-ai.db

# Vault Paths (adjust to your actual paths)
VAULT_PERSONAL_PATH=/vaults/personal
VAULT_WORK_PATH=/vaults/work

# Qdrant Configuration (uses service name from docker-compose)
QDRANT_URL=http://qdrant:6333
QDRANT_COLLECTION=notes
QDRANT_VECTOR_SIZE=1024

# API Configuration
API_PORT=9000
LOG_LEVEL=INFO
LOG_FORMAT=json
```

### 4. Update docker-compose.yml

Edit `docker-compose.yml` and:

1. Update the image reference to match your GitHub repository:
   ```yaml
   image: ghcr.io/your-username/helloworld-ai:latest
   ```

2. Update the volume mounts for your vaults:
   ```yaml
   volumes:
     - /path/to/your/personal/vault:/vaults/personal:ro
     - /path/to/your/work/vault:/vaults/work:ro
   ```

### 5. Authenticate with GitHub Container Registry

If your repository is private, authenticate Docker:

```bash
# Create a GitHub Personal Access Token with 'read:packages' permission
# Then login:
echo $GITHUB_TOKEN | docker login ghcr.io -u YOUR_USERNAME --password-stdin
```

### 6. Set Up Polling Deployment

Set up a cron job to check for updates periodically:

```bash
# Edit crontab
crontab -e

# Add this line to check every 5 minutes:
*/5 * * * * cd ~/helloworld-ai && GITHUB_REPOSITORY="your-username/helloworld-ai" ./scripts/deploy-poll.sh >> ~/helloworld-ai/deploy.log 2>&1

# Or check every 15 minutes:
*/15 * * * * cd ~/helloworld-ai && GITHUB_REPOSITORY="your-username/helloworld-ai" ./scripts/deploy-poll.sh >> ~/helloworld-ai/deploy.log 2>&1
```

### 7. Initial Deployment

Test the deployment manually first:

```bash
cd ~/helloworld-ai
export GITHUB_REPOSITORY="your-username/helloworld-ai"
./scripts/deploy-poll.sh
```

This will:
- Check for new images
- Pull the latest image from GHCR
- Start Qdrant and API containers
- Wait for health checks
- Show container status

### 8. Verify Deployment

Check that containers are running:

```bash
docker ps
docker logs helloworld-ai-api
docker logs helloworld-ai-qdrant
```

Test the API:

```bash
curl http://localhost:9000/health
```

## How Polling Works

The `scripts/deploy-poll.sh` script:
- Compares the current running container's image digest with the latest in GHCR
- If a new image is available, pulls it and updates the container
- Performs graceful rolling updates with health checks
- Logs all activity with timestamps

## Container Architecture

- **API**: Go service (port 9000) - built by this repo, runs in Docker
- **Qdrant**: Vector database (port 6333) - runs in Docker via docker-compose
- **llama.cpp**: LLM server (port 8081) - runs separately on host, NOT in Docker

The API container connects to:
- Qdrant via service name `qdrant:6333` (same Docker network)
- llama.cpp via `host.docker.internal:8081` (if llama.cpp is on the host)

## Troubleshooting

### llama.cpp Connection Issues

If the API can't connect to llama.cpp:
- Verify llama.cpp server is running: `curl http://localhost:8081/models`
- Check `LLM_BASE_URL` in your `.env` file
- For Docker, use `host.docker.internal:8081` if llama.cpp is on the host
- Ensure llama.cpp is bound to `0.0.0.0` not just `127.0.0.1`

### Docker Image Pull Fails

```bash
# Login to GHCR manually
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Pull image manually
docker pull ghcr.io/your-username/helloworld-ai:latest
```

### Container Health Check Fails

```bash
# Check container logs
docker logs helloworld-ai-api
docker logs helloworld-ai-qdrant

# Check container status
docker inspect helloworld-ai-api | grep -A 10 Health
```

### Vault Path Issues

Ensure vault paths in `docker-compose.yml` are:
- Absolute paths (not relative)
- Accessible by Docker
- Mounted as read-only (`:ro`)

## Maintenance

### Updating Configuration

1. Edit `.env` on production server
2. Restart containers: `docker-compose restart api`

### Viewing Logs

```bash
# API logs
docker logs -f helloworld-ai-api

# Qdrant logs
docker logs -f helloworld-ai-qdrant

# All logs
docker-compose logs -f

# Polling script logs (if using cron)
tail -f ~/helloworld-ai/deploy.log
```

### Manual Update

You can manually trigger an update check:

```bash
cd ~/helloworld-ai
export GITHUB_REPOSITORY="your-username/helloworld-ai"
./scripts/deploy-poll.sh
```

## Next Steps

- Set up monitoring/alerting for deployment failures
- Configure backup strategy for SQLite database
- Set up log aggregation (optional)
- Consider adding deployment notifications (Slack, email, etc.)
