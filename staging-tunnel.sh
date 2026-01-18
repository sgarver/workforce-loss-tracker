#!/bin/bash
# Staging Tunnel Script for Workforce Loss Tracker
# Usage: ./staging-tunnel.sh
# This creates an SSH tunnel to access the staging environment at http://localhost:3000

# Configuration
REMOTE_HOST="REDACTED_SERVER_IP"
REMOTE_USER="linuxuser"
LOCAL_PORT=3000
REMOTE_PORT=3000

echo "🚀 Starting SSH tunnel to staging environment..."
echo "📍 Remote: $REMOTE_USER@$REMOTE_HOST"
echo "🔗 Local port: $LOCAL_PORT → Remote port: $REMOTE_PORT"
echo "🌐 Access staging at: http://localhost:$LOCAL_PORT"
echo "❌ Press Ctrl+C to stop the tunnel"
echo ""

# Start the SSH tunnel
ssh -L $LOCAL_PORT:localhost:$REMOTE_PORT $REMOTE_USER@$REMOTE_HOST

echo ""
echo "✅ Tunnel closed. Staging environment no longer accessible locally."