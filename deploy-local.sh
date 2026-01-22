#!/bin/bash
set -e

echo "🚀 Starting improved local deployment..."

# Configuration - can be overridden with environment variables
SERVER_HOST="${SERVER_HOST:-workforceloss.com}"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/github_actions_key}"
GITHUB_REPO="${GITHUB_REPO:-sgarver/workforce-loss-tracker}"

# Allow domain name for SERVER_HOST
if [[ "$SERVER_HOST" != *:* ]]; then
    echo "📝 Using domain name: $SERVER_HOST"
    SSH_TARGET="linuxuser@$SERVER_HOST"
    SCP_TARGET="$SSH_TARGET"
else
    echo "📝 Using IPv6 address: $SERVER_HOST"
    SSH_TARGET="linuxuser@[$SERVER_HOST]"
    SCP_TARGET="$SSH_TARGET"
fi

echo "🔧 Configuration:"
echo "   Server: $SERVER_HOST"
echo "   SSH Key: $SSH_KEY_PATH"
echo "   Repository: $GITHUB_REPO"
echo ""

# Pre-deployment checks
echo "🔍 Running pre-deployment checks..."

# Check if we have required files
if [ ! -f "go.mod" ]; then
    echo "❌ Not in project root directory (go.mod not found)"
    exit 1
fi

# Check SSH key exists
if [ ! -f "$SSH_KEY_PATH" ]; then
    echo "❌ SSH key not found at $SSH_KEY_PATH"
    echo "   Please ensure your SSH key exists or set SSH_KEY_PATH environment variable"
    exit 1
fi

# Test SSH connection
echo "🔐 Testing SSH connection..."
if ! ssh -o ConnectTimeout=10 -o BatchMode=yes "$SSH_TARGET" "echo 'SSH connection successful'" >/dev/null 2>&1; then
    echo "❌ SSH connection failed. Please check your SSH key and server access."
    exit 1
fi
echo "✅ SSH connection verified"

# Function to check if binary is available
check_binary() {
    if [ -f "layoff-tracker" ] && [ -x "layoff-tracker" ]; then
        echo "✅ Local binary found"
        return 0
    else
        echo "❌ No local binary found"
        return 1
    fi
}

# Function to build binary locally
build_binary() {
    echo "🔨 Building binary locally..."
    if go build -o layoff-tracker .; then
        echo "✅ Binary built successfully"
        return 0
    else
        echo "❌ Local build failed"
        return 1
    fi
}

# Try to use CI artifact first, fall back to local build
BINARY_READY=false

# Check for CI artifacts
if command -v gh &> /dev/null && gh auth status &> /dev/null 2>&1; then
    echo "🔍 Checking for CI artifacts..."

    # Get latest successful run
    RUN_ID=$(gh run list --workflow="Production CI" --repo="$GITHUB_REPO" --status=success --limit=1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)

    if [ -n "$RUN_ID" ]; then
        echo "📦 Found CI run $RUN_ID"

        # Get commit SHA
        RUN_SHA=$(gh run view "$RUN_ID" --repo="$GITHUB_REPO" --json headSha --jq '.headSha' 2>/dev/null || true)

        if [ -n "$RUN_SHA" ]; then
            ARTIFACT_NAME="layoff-tracker-$RUN_SHA"

            # Get artifact ID
            ARTIFACT_ID=$(gh api "/repos/$GITHUB_REPO/actions/runs/$RUN_ID/artifacts" --jq '.artifacts[] | select(.name == "'"$ARTIFACT_NAME"'") | .id' 2>/dev/null || true)

            if [ -n "$ARTIFACT_ID" ]; then
                echo "📥 Downloading CI artifact..."

                # Create temp directory
                TEMP_DIR=$(mktemp -d)
                trap "rm -rf $TEMP_DIR" EXIT

                # Download artifact
                if gh api "/repos/$GITHUB_REPO/actions/artifacts/$ARTIFACT_ID/zip" > "$TEMP_DIR/artifact.zip" 2>/dev/null; then
                    # Extract binary
                    if unzip -q "$TEMP_DIR/artifact.zip" -d "$TEMP_DIR" 2>/dev/null; then
                        BINARY_PATH=$(find "$TEMP_DIR" -name "layoff-tracker" -type f | head -1)
                        if [ -n "$BINARY_PATH" ]; then
                            cp "$BINARY_PATH" ./layoff-tracker
                            chmod +x ./layoff-tracker
                            echo "✅ CI artifact downloaded and ready"
                            BINARY_READY=true
                        fi
                    fi
                fi
                rm -rf "$TEMP_DIR"
            fi
        fi
    fi
fi

# Fall back to local build if CI artifact not available
if [ "$BINARY_READY" = false ]; then
    echo "📦 CI artifact not available, building locally..."
    if build_binary; then
        BINARY_READY=true
    else
        echo "❌ Failed to prepare binary"
        exit 1
    fi
fi

# Upload binary
echo "📤 Uploading binary to server..."
if ! scp layoff-tracker "$SCP_TARGET:/tmp/"; then
    echo "❌ Failed to upload binary to server"
    exit 1
fi

# Upload templates
echo "📄 Uploading templates..."
if [ -d "templates" ]; then
    if ! scp -r templates "$SCP_TARGET:/tmp/"; then
        echo "⚠️  Warning: Failed to upload templates"
    fi
else
    echo "⚠️  Warning: No templates directory found"
fi

# Upload static files
echo "🖼️  Uploading static assets..."
if [ -d "static" ]; then
    if ! scp -r static "$SCP_TARGET:/tmp/"; then
        echo "⚠️  Warning: Failed to upload static files"
    fi
else
    echo "⚠️  Warning: No static directory found"
fi

# Execute deployment on server
echo "🔄 Executing deployment on server..."
if ! ssh "$SSH_TARGET" << 'EOF'
echo "📋 Starting server deployment..."

# Stop service (with timeout)
echo "⏹️  Stopping service..."
sudo systemctl stop layoff-tracker || true

# Wait for service to fully stop
sleep 2

# Create backup
BACKUP_FILE="/opt/layoff-tracker/layoff-tracker.backup.$(date +%Y%m%d_%H%M%S)"
if [ -f /opt/layoff-tracker/layoff-tracker ]; then
    cp /opt/layoff-tracker/layoff-tracker "$BACKUP_FILE"
    echo "💾 Backup created: $BACKUP_FILE"
fi

# Deploy new binary
cp /tmp/layoff-tracker /opt/layoff-tracker/layoff-tracker
echo "📦 New binary deployed"

# Deploy templates and static files
if [ -d "/tmp/templates" ]; then
    cp -r /tmp/templates/* /opt/layoff-tracker/templates/ 2>/dev/null || echo "⚠️  Some template files may not have copied"
    echo "📄 Templates deployed"
fi
if [ -d "/tmp/static" ]; then
    cp -r /tmp/static/* /opt/layoff-tracker/static/ 2>/dev/null || echo "⚠️  Some static files may not have copied"
    echo "🖼️  Static files deployed"
fi

# Set proper permissions
chmod 755 /opt/layoff-tracker/layoff-tracker

# Start service
echo "▶️  Starting service..."
sudo systemctl start layoff-tracker

# Health check
echo "🏥 Running health check..."
sleep 5
if curl -f -s http://localhost:8080/ping > /dev/null 2>&1; then
    echo "✅ Basic health check passed"
else
    echo "❌ Health check failed! Rolling back..."
    sudo systemctl stop layoff-tracker
    if [ -f "$BACKUP_FILE" ]; then
        cp "$BACKUP_FILE" /opt/layoff-tracker/layoff-tracker
        sudo systemctl start layoff-tracker
        echo "🔄 Rollback completed"
    fi
    exit 1
fi

# Asset verification
echo "🔍 Verifying deployment assets..."
MISSING_ASSETS=()
if [ ! -x "/opt/layoff-tracker/layoff-tracker" ]; then
    MISSING_ASSETS+=("layoff-tracker binary")
fi
if [ ! -f "/opt/layoff-tracker/templates/dashboard.html" ]; then
    MISSING_ASSETS+=("dashboard.html template")
fi
if [ ! -f "/opt/layoff-tracker/templates/layout.html" ]; then
    MISSING_ASSETS+=("layout.html template")
fi

if [ ${#MISSING_ASSETS[@]} -gt 0 ]; then
    echo "❌ Missing assets: ${MISSING_ASSETS[*]}"
    exit 1
fi
echo "✅ All critical assets verified"

# API functionality check
echo "🔗 Testing API functionality..."
if curl -f -s http://localhost:8080/api/stats | jq '.company_breakdown | length' > /dev/null 2>&1; then
    echo "✅ API responding correctly"
else
    echo "⚠️  Warning: API not returning expected data"
fi

echo "🎉 Deployment completed successfully!"
EOF
then
    echo "❌ SSH deployment failed"
    exit 1
fi

# Cleanup
echo "🧹 Cleaning up..."
rm -f layoff-tracker

echo "✅ Local deployment process completed successfully!"