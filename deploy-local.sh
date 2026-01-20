#!/bin/bash
set -e

echo "🚀 Starting local deployment..."

# Check if GitHub CLI is installed
if ! command -v gh &> /dev/null; then
    echo "❌ GitHub CLI (gh) is required. Install from: https://cli.github.com/"
    exit 1
fi

# Check if user is logged in to GitHub
if ! gh auth status &> /dev/null; then
    echo "❌ Please login to GitHub CLI: gh auth login"
    exit 1
fi

# Get the latest successful workflow run
echo "📦 Finding latest successful CI run..."
RUN_ID=$(gh run list --workflow="Production CI" --status=success --limit=1 --json databaseId --jq '.[0].databaseId')

if [ -z "$RUN_ID" ]; then
    echo "❌ No successful CI runs found. Please ensure CI has passed."
    exit 1
fi

echo "📥 Downloading artifact from run $RUN_ID..."
gh run download $RUN_ID -n "layoff-tracker-${GITHUB_SHA:-latest}"

if [ ! -f "layoff-tracker" ]; then
    echo "❌ Artifact download failed or file not found."
    exit 1
fi

echo "🔐 Setting up SSH..."
mkdir -p ~/.ssh
# Note: You'll need to set your SSH key path
SSH_KEY_PATH="${SSH_KEY_PATH:-~/.ssh/github_actions_key}"
if [ ! -f "$SSH_KEY_PATH" ]; then
    echo "❌ SSH key not found at $SSH_KEY_PATH"
    echo "Please set SSH_KEY_PATH environment variable or ensure key exists."
    exit 1
fi

# Copy key and set permissions
cp "$SSH_KEY_PATH" ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa

# Add server to known hosts (you'll need to set SERVER_HOST)
SERVER_HOST="${SERVER_HOST:-2001:19f0:5400:2f1e:5400:05ff:fee4:2ad6}"
ssh-keyscan -H "$SERVER_HOST" >> ~/.ssh/known_hosts

echo "🚀 Deploying to production server..."
scp layoff-tracker linuxuser@[$SERVER_HOST]:/tmp/

ssh linuxuser@[$SERVER_HOST] << 'EOF'
echo "📋 Starting server deployment..."

# Stop service
sudo systemctl stop layoff-tracker

# Create backup
BACKUP_FILE="/opt/layoff-tracker/layoff-tracker.backup.$(date +%Y%m%d_%H%M%S)"
cp /opt/layoff-tracker/layoff-tracker "$BACKUP_FILE"
echo "💾 Backup created: $BACKUP_FILE"

# Deploy new binary
cp /tmp/layoff-tracker /opt/layoff-tracker/layoff-tracker
echo "📦 New binary deployed"

# Start service
sudo systemctl start layoff-tracker
echo "▶️  Service started"

# Health check
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

echo "🧹 Cleaning up..."
rm -f layoff-tracker

echo "✅ Local deployment process completed!"