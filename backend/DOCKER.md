# Docker Setup Guide

This project includes Docker support for easy deployment and local development.

## Files

- **Dockerfile** - Multi-stage build using Alpine Linux for minimal image size
- **docker-compose.yml** - Production-ready composition with PostgreSQL and file-server
- **docker-compose.override.yml** - Development overrides (auto-loaded by docker-compose)
- **.env.docker** - Environment variables template

## Prerequisites

- Docker Engine 20.10+
- Docker Compose 1.29+
- AWS S3 credentials (for file storage)

## Quick Start

### 1. Setup Environment Variables

Copy and configure the environment file:

```bash
cp .env.docker .env.local
# Edit .env.local with your AWS credentials and configuration
```

Required AWS variables:
- `AWS_ACCESS_KEY_ID` - Your AWS access key
- `AWS_SECRET_ACCESS_KEY` - Your AWS secret key
- `S3_BUCKET` - Your S3 bucket name
- `S3_PREFIX` - Path prefix in bucket (default: `uploads/`)

### 2. Start Services

```bash
# Start both PostgreSQL and file-server
docker-compose --env-file .env.local up -d

# View logs
docker-compose logs -f server

# Stop services
docker-compose down
```

### 3. Verify Setup

```bash
# Check health
curl http://localhost:8080/health

# View running containers
docker-compose ps

# Check database connection
docker-compose exec postgres psql -U postgres -d file_server -c "\dt"
```

## Building the Image

```bash
# Build the image
docker build -t file-server:latest .

# Build with custom tag
docker build -t file-server:1.0.0 .
```

## Environment Variables

### Database
- `DB_USER` - PostgreSQL user (default: postgres)
- `DB_PASSWORD` - PostgreSQL password (default: postgres)
- `DB_NAME` - Database name (default: file_server)
- `DB_PORT` - PostgreSQL port (default: 5432)
- `DATABASE_URL` - Full connection string (auto-generated in docker-compose)

### API Server
- `API_PORT` - Server port (default: 8080)
- `MAX_FILE_SIZE_MB` - Max file size in MB (default: 500)
- `MAX_TOTAL_UPLOAD_MB` - Max total uploads in MB (default: 10000)
- `DOWNLOAD_URL_EXPIRY_DAYS` - Download link expiry (default: 7)

### AWS S3
- `AWS_REGION` - AWS region (default: us-east-1)
- `AWS_ACCESS_KEY_ID` - Your AWS access key ID
- `AWS_SECRET_ACCESS_KEY` - Your AWS secret access key
- `S3_BUCKET` - S3 bucket name
- `S3_PREFIX` - Prefix for uploaded files (default: uploads/)

## Development

### Local Development with Docker

For live reloading during development:

1. Build the image once:
```bash
docker build -t file-server:dev .
```

2. Run with volume mounting:
```bash
docker run -it --rm \
  -p 8080:8080 \
  -v $(pwd):/app \
  -e DATABASE_URL="postgresql://postgres:postgres@host.docker.internal:5432/file_server" \
  file-server:dev \
  go run ./cmd/server
```

Or with docker-compose, uncomment the volumes in docker-compose.override.yml:
```bash
docker-compose --env-file .env.local up server
```

### Running Migrations Only

```bash
# Run migrations container
docker-compose run --rm server ./migrate
```

## Docker Compose Services

### PostgreSQL Service
- **Image**: postgres:16-alpine
- **Container**: file-server-postgres
- **Port**: 5432 (configurable via DB_PORT)
- **Volume**: postgres_data (persisted)
- **Healthcheck**: Enabled (10s interval)

### File-Server Service
- **Image**: Built from Dockerfile
- **Container**: file-server-app
- **Port**: 8080 (configurable via API_PORT)
- **Dependencies**: Waits for PostgreSQL healthcheck
- **Healthcheck**: Enabled (30s interval)
- **Startup**: Automatically runs migrations before starting

## Networking

Both services connect via `file-server-net` bridge network. Internal hostname:
- PostgreSQL: `postgres:5432`
- API Server: `server:8080`

## Database Persistence

PostgreSQL data is stored in the `postgres_data` volume. To preserve data:

```bash
# List volumes
docker volume ls

# Inspect volume
docker volume inspect file-server-postgres_postgres_data

# Backup database
docker-compose exec postgres pg_dump -U postgres file_server > backup.sql

# Restore database
docker-compose exec -T postgres psql -U postgres file_server < backup.sql
```

## Production Deployment

### Building for Production

```bash
# Build optimized image
docker build -t file-server:prod --target=final .

# Tag for registry
docker tag file-server:prod myregistry.com/file-server:1.0.0

# Push to registry
docker push myregistry.com/file-server:1.0.0
```

### Using Cloud Managed Databases (Recommended)

For production, use managed PostgreSQL (RDS, Cloud SQL, etc.):

```bash
# Set DATABASE_URL to your managed database
export DATABASE_URL=postgresql://user:pass@cloud-db.example.com:5432/file_server

# Run container without postgres service
docker run -p 8080:8080 \
  -e DATABASE_URL="$DATABASE_URL" \
  -e AWS_ACCESS_KEY_ID="your_key" \
  -e AWS_SECRET_ACCESS_KEY="your_secret" \
  -e S3_BUCKET="your_bucket" \
  file-server:prod
```

### Docker Swarm / Kubernetes

Dockerfile is compatible with container orchestration platforms:

```bash
# For Kubernetes
kubectl apply -f k8s-deployment.yaml

# For Docker Swarm
docker stack deploy -c docker-compose.yml file-server
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker-compose logs server

# Check database connectivity
docker-compose logs postgres
```

### Database connection failed

```bash
# Verify PostgreSQL is running
docker-compose ps

# Wait for healthcheck
docker-compose ps  # Check STATUS column

# Check network
docker network inspect file-server-postgres_file-server-net
```

### Permission denied errors

```bash
# Run migrations with proper permissions
docker-compose exec server ./migrate

# Check volume permissions
docker-compose exec postgres ls -la /var/lib/postgresql/data
```

### Port already in use

```bash
# Change port in .env.local
echo "API_PORT=9000" >> .env.local
echo "DB_PORT=5433" >> .env.local

docker-compose --env-file .env.local up -d
```

## Cleanup

```bash
# Stop and remove containers
docker-compose down

# Remove volumes (WARNING: deletes data)
docker-compose down -v

# Remove images
docker rmi file-server:latest

# Prune unused resources
docker system prune -a
```

## Performance Notes

- **Alpine Linux**: ~15MB base image for minimal size
- **Multi-stage build**: Reduces final image by separating build and runtime
- **Database healthcheck**: Ensures API only starts after DB is ready
- **Container restart policy**: `unless-stopped` for production reliability

## Resources

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Specification](https://docs.docker.com/compose/compose-file/)
- [Alpine Linux](https://www.alpinelinux.org/)
- [PostgreSQL Docker Image](https://hub.docker.com/_/postgres)
