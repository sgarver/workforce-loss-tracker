# Deployment Guide

## Production Deployment

Deployments are handled via GitHub Actions workflow_dispatch.

1. **Trigger Deploy:**
   - Go to [Actions](https://github.com/sgarver/workforce-loss-tracker/actions)
   - Select "CI/CD Pipeline"
   - Click "Run workflow"
   - Select `main` branch
   - Choose `production` environment
   - Run (admin only)

2. **Process:**
   - CI validates code (tests, security, build)
   - Binary uploaded to production server
   - Service restarted with backup creation

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

Push to `staging` branch → CI runs → Manual review → Merge to `main` for production.

## Monitoring

- **Health Check:** https://workforceloss.com/ping
- **CI Status:** Check README badge or Actions tab
- **Logs:** Server logs via `sudo journalctl -u layoff-tracker`