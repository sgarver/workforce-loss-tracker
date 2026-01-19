#!/bin/bash
# Local Development Test Script
# Run this before deploying to production to ensure everything works locally

set -e

echo "🚀 Running local development tests..."

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[TEST] $1${NC}"; }
error() { echo -e "${RED}[ERROR] $1${NC}"; exit 1; }
warn() { echo -e "${YELLOW}[WARN] $1${NC}"; }

# Check Go installation
log "Checking Go installation..."
if ! command -v go &> /dev/null; then
    error "Go is not installed. Please install Go 1.21+"
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
if [[ "$(printf '%s\n' "$GO_VERSION" "1.21" | sort -V | head -n1)" != "1.21" ]]; then
    error "Go version $GO_VERSION is too old. Please upgrade to Go 1.21+"
fi
log "✅ Go $GO_VERSION installed"

# Check dependencies
log "Checking dependencies..."
go mod tidy
log "✅ Dependencies resolved"

# Run tests
log "Running tests..."
if go test ./... -v; then
    log "✅ All tests passed"
else
    error "❌ Some tests failed. Please fix before deploying."
fi

# Build application
log "Building application..."
export CGO_ENABLED=1 GOOS=linux GOARCH=amd64
if go build -o layoff-tracker-test .; then
    log "✅ Build successful"
else
    error "❌ Build failed"
fi

# Test the build
log "Testing built application..."
./layoff-tracker-test &
APP_PID=$!
sleep 3

if curl -s http://localhost:8080/ping > /dev/null 2>&1; then
    log "✅ Application health check passed"
else
    kill $APP_PID 2>/dev/null || true
    error "❌ Application health check failed"
fi

# Clean up
kill $APP_PID 2>/dev/null || true
rm layoff-tracker-test
log "✅ Cleanup completed"

# Check for common issues
log "Checking for common issues..."

if [[ -f ".env" ]] && grep -q "your-" .env; then
    warn "⚠️  .env contains placeholder values. Make sure production .env is configured."
fi

if [[ -f ".server-config" ]]; then
    log "✅ Server config found"
else
    warn "⚠️  .server-config not found. Create it with server details."
fi

# Check Git status
if [[ -n $(git status --porcelain) ]]; then
    warn "⚠️  You have uncommitted changes. Consider committing or stashing them."
else
    log "✅ Git working directory clean"
fi

log "🎉 All local tests passed! Ready for production deployment."
log ""
log "Next steps:"
log "1. Commit any remaining changes: git add . && git commit -m 'message'"
log "2. Push to GitHub: git push origin main"
log "3. Deploy to production: ./deploy.sh"
log ""
log "Remember: Always test locally before deploying to production!"