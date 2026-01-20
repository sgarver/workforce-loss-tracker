# Deployment Guide

## Production Deployment

Deployments use a hybrid approach: automated CI on GitHub + manual local deployment.

### Automated CI Pipeline
1. **Automatic CI:** Push to `main` → GitHub runs tests, security scans, and builds binary
2. **Manual CI:** Optional - trigger "Production CI" workflow for validation without push

### Local Deployment
1. **Prerequisites:**
   - GitHub CLI installed: `gh auth login`
   - SSH key at `~/.ssh/github_actions_key` (or set `SSH_KEY_PATH`)
   - Set environment variables:
     ```bash
     export SERVER_HOST="2001:19f0:5400:2f1e:5400:05ff:fee4:2ad6"
     export GITHUB_SHA="latest"  # or specific commit SHA
     ```

2. **Run Deployment:**
   ```bash
   ./deploy-local.sh
   ```

3. **Process:**
   - Downloads latest successful build artifact
   - SCP binary to server
   - SSH executes deployment (backup, update, restart, health check)
   - Automatic rollback on failure

## Rollback

If deployment causes issues:

### Automatic Rollback
The deploy script automatically rolls back to the previous version if health check fails.

### Manual Rollback (Server Side)
```bash
# SSH to production server
ssh linuxuser@[2001:19f0:5400:2f1e:5400:05ff:fee4:2ad6]

# Go to app directory
cd /opt/layoff-tracker

# List backups
ls -la layoff-tracker.backup.*

# Restore latest backup (replace TIMESTAMP)
cp layoff-tracker.backup.TIMESTAMP layoff-tracker

# Restart service
sudo systemctl restart layoff-tracker

# Verify
curl http://localhost:8080/ping
```

### Emergency Rollback
If service won't start, check logs:
```bash
sudo journalctl -u layoff-tracker -n 50
```

## Staging Deployment

Push to `staging` branch → "Staging CI" workflow runs tests/security/build → Manual review → Merge to `main` for production.

## Monitoring

- **Health Check:** https://workforceloss.com/ping
- **CI Status:** Check README badge or Actions tab
- **Logs:** Server logs via `sudo journalctl -u layoff-tracker`