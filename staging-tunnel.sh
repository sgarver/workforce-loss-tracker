#!/bin/bash
# Staging Tunnel Script for Workforce Loss Tracker
# Usage: ./staging-tunnel.sh
# This creates an SSH tunnel to access the staging environment at http://localhost:3000

# Load configuration
if [[ -f ".server-config" ]]; then
    source .server-config
else
    echo "❌ Error: .server-config file not found!"
    echo "Please create .server-config with your server details:"
    echo "REMOTE_HOST=your-server-ip"
    echo "REMOTE_USER=your-username"
    echo "STAGING_LOCAL_PORT=3000"
    echo "STAGING_REMOTE_PORT=3000"
    exit 1
fi

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