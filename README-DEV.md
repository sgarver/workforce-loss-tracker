# Local Development Setup

## Development Workflow

**ALWAYS develop and test locally before deploying to production.**

### 1. Local Development Setup

#### Prerequisites
- Go 1.21+
- SQLite3 (for database)
- Git

#### Initial Setup
```bash
# Clone repository
git clone https://github.com/sgarver/workforce-loss-tracker.git
cd workforce-loss-tracker

# Install dependencies
go mod tidy

# Set up local environment
cp .server-config .server-config.local  # Edit for local settings
```

#### Database Setup
```bash
# Create local database
sqlite3 layoff_tracker.db < schema.sql  # If schema file exists

# Or let the app create it on first run
```

### 2. Local Development Commands

#### Start Development Server
```bash
# Standard mode
go run main.go

# Or with custom port
PORT=3000 go run main.go
```

#### Test the Application
```bash
# Health check
curl http://localhost:8080/ping

# View in browser
open http://localhost:8080
```

#### Run Tests
```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...
```

### 3. Development Workflow

#### Feature Development
```bash
# Create feature branch
git checkout -b feature/new-feature

# Make changes
# Edit code, templates, etc.

# Test locally
go run main.go
# Test in browser

# Commit changes
git add .
git commit -m "Add new feature"

# Merge to main
git checkout main
git merge feature/new-feature
```

#### Before Production Deployment
```bash
# Ensure all tests pass
go test ./...

# Build for production
export CGO_ENABLED=1 GOOS=linux GOARCH=amd64
go build -o layoff-tracker .

# Test build
./layoff-tracker &
sleep 3
curl http://localhost:8080/ping
pkill layoff-tracker

# If working, deploy
./deploy.sh
```

### 4. Local Configuration

#### Environment Variables
Create `.env` for local development:
```bash
# Local .env
GO_ENV=development
PORT=8080
BASE_URL=http://localhost:8080

# Use test OAuth credentials for local development
GOOGLE_CLIENT_ID=your-test-client-id
GOOGLE_CLIENT_SECRET=your-test-secret

DATABASE_PATH=layoff_tracker.db
```

#### Local OAuth Setup
For local testing, create test OAuth credentials in Google Console:
- Authorized redirect URIs: `http://localhost:8080/auth/google/callback`
- Use test credentials in local `.env`

### 5. Deployment Checklist

**Before deploying to production:**

- [ ] All local tests pass (`go test ./...`)
- [ ] Application runs locally without errors
- [ ] No console errors in browser dev tools
- [ ] Theme switching works correctly
- [ ] All pages load properly
- [ ] OAuth authentication works (if implemented)
- [ ] Database operations work correctly

**If all checks pass:**
```bash
./deploy.sh  # Deploy to production
```

### 6. Troubleshooting Local Development

#### Common Issues
```bash
# Port already in use
lsof -i :8080
kill -9 <PID>

# Database issues
rm layoff_tracker.db  # Reset database

# Go module issues
go mod tidy
go clean -modcache
```

#### Logs
```bash
# View application logs
go run main.go  # Logs appear in terminal

# Debug mode
DEBUG=1 go run main.go
```

### 7. Production vs Local Differences

| Aspect | Local | Production |
|--------|-------|------------|
| Port | 8080 | 80/443 |
| Domain | localhost | workforceloss.com |
| SSL | No | Yes (Let's Encrypt) |
| Database | Local SQLite | Server SQLite |
| OAuth | Test credentials | Production credentials |
| Environment | Development | Production |

## Development Best Practices

- **Always test locally first**
- **Commit frequently with clear messages**
- **Run tests before committing**
- **Use feature branches for new work**
- **Keep local .env out of version control**
- **Document any setup changes**

This workflow ensures stable, tested deployments to production.
