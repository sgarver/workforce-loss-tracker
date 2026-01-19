# Deployment Guide

## Production Deployment

Deployments are handled via GitHub Actions workflow_dispatch.

1. **Trigger Deploy:**
    - Go to [Actions](https://github.com/sgarver/workforce-loss-tracker/actions)
    - Select "Production Deploy"
    - Click "Run workflow"
    - Branch: `main`
    - Action: `deploy`
    - Run (admin only - sgarver)

2. **Process:**
    - Workflow builds Linux binary
    - Deploys to production server via SSH
    - Creates timestamped backup before update
    - Restarts systemd service
    - Verifies health check (ping endpoint)

## Rollback

If production deployment causes issues:

### Quick Rollback (Server Side)
```bash
# SSH to production server
ssh linuxuser@workforceloss.com

# Go to app directory
cd /opt/layoff-tracker

# List backups
ls -la layoff-tracker.backup.*

# Restore latest backup (replace TIMESTAMP)
cp layoff-tracker.backup.TIMESTAMP layoff-tracker

# Restart service
sudo systemctl restart layoff-tracker

# Verify
curl https://workforceloss.com/ping
```

### Emergency Rollback
If service won't start, check logs:
```bash
sudo journalctl -u layoff-tracker -n 50
```

Contact admin for assistance.

## Staging Deployment

Push to `staging` branch → "Staging CI" workflow runs tests/security/build → Manual review → Merge to `main` for production.

## Monitoring

- **Health Check:** https://workforceloss.com/ping
- **CI Status:** Check README badge or Actions tab
- **Logs:** Server logs via `sudo journalctl -u layoff-tracker`