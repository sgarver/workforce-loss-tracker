#!/bin/bash
set -e

echo "🚀 Starting local deployment..."

# Configuration - can be overridden with environment variables
SERVER_HOST="${SERVER_HOST:-2001:19f0:5400:2f1e:5400:05ff:fee4:2ad6}"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/github_actions_key}"
GITHUB_REPO="${GITHUB_REPO:-sgarver/workforce-loss-tracker}"

# Allow domain name for SERVER_HOST
if [[ "$SERVER_HOST" != *:* ]]; then
    echo "📝 Using domain name: $SERVER_HOST"
    SSH_TARGET="linuxuser@$SERVER_HOST"
    SCP_TARGET="$SSH_TARGET"
else
    echo "📝 Using IPv6 address: $SERVER_HOST"
    SSH_TARGET="linuxuser@$SERVER_HOST"
    SCP_TARGET="$SSH_TARGET"
fi

echo "🔧 Configuration:"
echo "   Server: $SERVER_HOST"
echo "   SSH Key: $SSH_KEY_PATH"
echo "   Repository: $GITHUB_REPO"
echo ""

# Check if GitHub CLI is installed
if ! command -v gh &> /dev/null; then
    echo "❌ GitHub CLI (gh) is required. Install from: https://cli.github.com/"
    echo "   Or run: brew install gh  (on macOS)"
    exit 1
fi

# Check if user is logged in to GitHub
if ! gh auth status &> /dev/null; then
    echo "❌ Please login to GitHub CLI first:"
    echo "   gh auth login"
    exit 1
fi

# Get the latest successful workflow run
echo "📦 Finding latest successful CI run..."
RUN_ID=$(gh run list --workflow="Production CI" --repo="$GITHUB_REPO" --status=success --limit=1 --json databaseId --jq '.[0].databaseId')

if [ -z "$RUN_ID" ]; then
    echo "❌ No successful CI runs found. Please ensure CI has passed."
    exit 1
fi

echo "📥 Downloading artifact from run $RUN_ID..."
# Download to temp directory to avoid conflicts
TEMP_DIR=$(mktemp -d)
echo "Temp dir: $TEMP_DIR"
echo "Current dir before cd: $(pwd)"
cd "$TEMP_DIR"
echo "Current dir after cd: $(pwd)"
if ! gh run download "$RUN_ID" --repo="$GITHUB_REPO" -n "layoff-tracker-$(gh run view "$RUN_ID" --repo="$GITHUB_REPO" --json headSha --jq '.headSha')"; then
    echo "❌ Artifact download failed. Trying with latest naming..."
    gh run download "$RUN_ID" --repo="$GITHUB_REPO" 2>/dev/null || {
        cd - > /dev/null
        rm -rf "$TEMP_DIR"
        echo "❌ Could not find artifact. Make sure CI completed successfully."
        exit 1
    }
fi

if [ ! -f "layoff-tracker" ]; then
    cd - > /dev/null
    rm -rf "$TEMP_DIR"
    echo "❌ Binary file 'layoff-tracker' not found after download."
    exit 1
fi

# Copy binary back to project directory
cp layoff-tracker "$OLDPWD/"
cd - > /dev/null
rm -rf "$TEMP_DIR"

echo "📥 Downloading artifact from run $RUN_ID..."
if ! gh run download "$RUN_ID" --repo="$GITHUB_REPO" -n "layoff-tracker-$(gh run view "$RUN_ID" --repo="$GITHUB_REPO" --json headSha --jq '.headSha')"; then
    echo "❌ Artifact download failed. Trying with latest naming..."
    gh run download "$RUN_ID" --repo="$GITHUB_REPO" 2>/dev/null || {
        echo "❌ Could not find artifact. Make sure CI completed successfully."
        exit 1
    }
fi

if [ ! -f "layoff-tracker" ]; then
    echo "❌ Binary file 'layoff-tracker' not found after download."
    exit 1
fi

echo "🔐 Setting up SSH..."
mkdir -p ~/.ssh
if [ ! -f "$SSH_KEY_PATH" ]; then
    echo "❌ SSH key not found at $SSH_KEY_PATH"
    echo "   Please ensure your SSH key exists at that location,"
    echo "   or set SSH_KEY_PATH environment variable:"
    echo "   export SSH_KEY_PATH=/path/to/your/key"
    exit 1
fi

# Copy key and set permissions
cp "$SSH_KEY_PATH" ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa

# Add server to known hosts
echo "🔍 Adding server to known hosts..."
if ! ssh-keyscan -H "$SERVER_HOST" >> ~/.ssh/known_hosts 2>/dev/null; then
    echo "⚠️  Warning: Could not add server to known hosts (might be ok if using domain)"
fi

echo "🚀 Deploying to production server..."
echo "   Target: $SCP_TARGET"

if ! scp layoff-tracker "$SCP_TARGET:/tmp/"; then
    echo "❌ Failed to upload binary to server"
    exit 1
fi

echo "🔄 Executing deployment on server..."
if ! ssh "$SSH_TARGET" << 'EOF'
echo "📋 Starting server deployment..."

# Stop service
echo "⏹️  Stopping service..."
sudo systemctl stop layoff-tracker

# Create backup
BACKUP_FILE="/opt/layoff-tracker/layoff-tracker.backup.$(date +%Y%m%d_%H%M%S)"
cp /opt/layoff-tracker/layoff-tracker "$BACKUP_FILE"
echo "💾 Backup created: $BACKUP_FILE"

# Deploy new binary
cp /tmp/layoff-tracker /opt/layoff-tracker/layoff-tracker
echo "📦 New binary deployed"

# Start service
echo "▶️  Starting service..."
sudo systemctl start layoff-tracker

# Health check
echo "🏥 Running health check..."
sleep 5
if curl -f http://localhost:8080/ping > /dev/null 2>&1; then
    echo "✅ Deployment successful! Server is responding."
else
    echo "❌ Health check failed! Rolling back..."
    sudo systemctl stop layoff-tracker
    cp "$BACKUP_FILE" /opt/layoff-tracker/layoff-tracker
    sudo systemctl start layoff-tracker
    echo "🔄 Rollback completed"
    exit 1
fi

echo "🎉 Deployment completed successfully!"
EOF
then
    echo "❌ SSH deployment failed"
    exit 1
fi

echo "🧹 Cleaning up..."
rm -f layoff-tracker

echo "✅ Local deployment process completed successfully!"