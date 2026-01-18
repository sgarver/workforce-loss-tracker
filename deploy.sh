#!/bin/bash

# Workforce Loss Tracker Production Deployment Script
# Run as: linuxuser (with sudo access)
# Usage: chmod +x deploy.sh && ./deploy.sh

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

error() {
    echo -e "${RED}[ERROR] $1${NC}" >&2
}

warn() {
    echo -e "${YELLOW}[WARN] $1${NC}"
}

# Check if running as linuxuser
if [[ "$USER" != "linuxuser" ]]; then
    error "This script must be run as 'linuxuser'"
    exit 1
fi

log "Starting Workforce Loss Tracker deployment..."

# Step 3: Install Go Environment
log "Installing Go 1.21+..."
if ! command -v go &> /dev/null; then
    wget -q https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
    rm go1.21.5.linux-amd64.tar.gz
    
    # Add Go to PATH
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    export PATH=$PATH:/usr/local/go/bin
    log "Go installed successfully"
else
    log "Go already installed"
fi

go version

# Step 4: Prepare Application Directory
log "Setting up application directory..."
sudo mkdir -p /opt/layoff-tracker
sudo chown linuxuser:linuxuser /opt/layoff-tracker

if [[ -d "/home/linuxuser/tech-layoff-tracker" ]]; then
    log "Copying source code to production directory..."
    cp -r /home/linuxuser/tech-layoff-tracker/* /opt/layoff-tracker/
else
    error "Source code not found in /home/linuxuser/tech-layoff-tracker"
    error "Please upload source code first: scp -r /path/to/local/tech-layoff-tracker linuxuser@[ipv6]:/home/linuxuser/"
    exit 1
fi

cd /opt/layoff-tracker

# Step 5: Configure Environment Variables
log "Creating .env template..."
if [[ ! -f ".env" ]]; then
    cat > .env << 'EOF'
# Production Environment Variables
GO_ENV=production
PORT=8080
SESSION_SECRET=CHANGE_THIS_TO_A_SECURE_RANDOM_STRING
GOOGLE_CLIENT_ID=your-production-google-oauth-client-id
GOOGLE_CLIENT_SECRET=your-production-google-oauth-secret
DATABASE_PATH=/opt/layoff-tracker/layoff_tracker.db
BASE_URL=https://workforceloss.com
SMTP_HOST=localhost
SMTP_PORT=25
ADMIN_EMAIL=your-admin-email@example.com
EOF
    warn "Created .env template - EDIT WITH YOUR ACTUAL CREDENTIALS BEFORE CONTINUING"
    warn "Especially: SESSION_SECRET, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET"
    read -p "Press Enter after editing .env file..."
else
    log ".env file already exists"
fi

# Step 6: Check Binary and Test Application
log "Checking for application binary..."
if [[ -f "layoff-tracker" ]]; then
    log "Binary found, proceeding with tests..."
else
    error "Binary not found in /opt/layoff-tracker/"
    error "Please upload the compiled binary first"
    exit 1
fi

log "Testing application..."
# Try migration if available
./layoff-tracker --migrate 2>/dev/null || log "No migrate flag available, continuing..."

# Start app briefly for testing
timeout 10s ./layoff-tracker &
APP_PID=$!
sleep 3

if curl -s http://localhost:8080/ping > /dev/null; then
    log "Application test successful"
    kill $APP_PID 2>/dev/null || true
else
    error "Application test failed"
    kill $APP_PID 2>/dev/null || true
    exit 1
fi

# Step 7: Install and Configure Caddy
log "Installing and configuring Caddy..."
if ! command -v caddy &> /dev/null; then
    sudo apt update
    sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
    sudo apt update
    sudo apt install -y caddy
    log "Caddy installed"
else
    log "Caddy already installed"
fi

# Configure Caddyfile
sudo tee /etc/caddy/Caddyfile > /dev/null << 'EOF'
workforceloss.com {
    reverse_proxy localhost:8080
}
EOF

sudo systemctl enable caddy
sudo systemctl restart caddy
log "Caddy configured and started"

# Step 8: Create Systemd Service
log "Creating systemd service..."
sudo tee /etc/systemd/system/layoff-tracker.service > /dev/null << 'EOF'
[Unit]
Description=Workforce Loss Tracker
After=network.target

[Service]
Type=simple
User=linuxuser
WorkingDirectory=/opt/layoff-tracker
ExecStart=/opt/layoff-tracker/layoff-tracker
Environment=GO_ENV=production
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable layoff-tracker
sudo systemctl start layoff-tracker

# Check service status
if sudo systemctl is-active --quiet layoff-tracker; then
    log "Service started successfully"
else
    error "Service failed to start"
    sudo journalctl -u layoff-tracker --no-pager -n 20
    exit 1
fi

# Step 10: Final Verification
log "Running final tests..."
sleep 5

# Test local service
if curl -s http://localhost:8080/ping > /dev/null; then
    log "Local service test: PASSED"
else
    error "Local service test: FAILED"
fi

# Test through Caddy (will fail until DNS propagates)
if curl -s --max-time 5 https://workforceloss.com/ping > /dev/null 2>&1; then
    log "Public HTTPS test: PASSED"
else
    warn "Public HTTPS test: FAILED (expected until DNS propagates)"
fi

log "Deployment script completed!"
log "Next steps:"
log "1. Configure DNS: Add AAAA record in Porkbun -> REDACTED_SERVER_IP"
log "2. Wait 5-30 minutes for DNS propagation"
log "3. Test public access: curl https://workforceloss.com/ping"
log "4. Configure Google OAuth for https://workforceloss.com"
log "5. Optional: Run create_service_user.sh for dedicated user"

# Optional: Create dedicated service user script
log "Creating optional service user script..."
cat > /opt/layoff-tracker/create_service_user.sh << 'EOF'
#!/bin/bash
# Run this separately after deployment to create dedicated service user

sudo useradd -m -s /bin/bash layofftracker
sudo usermod -aG sudo layofftracker

sudo mkdir /home/layofftracker/.ssh
sudo cp ~/.ssh/authorized_keys /home/layofftracker/.ssh/
sudo chown -R layofftracker:layofftracker /home/layofftracker/.ssh
sudo chmod 700 /home/layofftracker/.ssh
sudo chmod 600 /home/layofftracker/.ssh/authorized_keys

# Update service
sudo sed -i 's/User=linuxuser/User=layofftracker/' /etc/systemd/system/layoff-tracker.service
sudo chown -R layofftracker:layofftracker /opt/layoff-tracker

sudo systemctl daemon-reload
sudo systemctl restart layoff-tracker

echo "Dedicated service user created. Test service status."
EOF

chmod +x /opt/layoff-tracker/create_service_user.sh
log "Optional service user script created at /opt/layoff-tracker/create_service_user.sh"