# Deployment Guide

## Overview

This application uses a **hybrid deployment approach**: automated CI/CD on GitHub for builds and testing, combined with manual local deployment for production releases.

**⚠️ Critical Note:** Always follow the complete deployment checklist. Recent deployments revealed missing assets (templates) that weren't caught by incomplete verification.

## Development Workflow

Follow this process to ensure smooth deployments and catch issues before they reach production:

### Process Overview
```
1. Develop → 2. Test Locally → 3. Merge to Main → 4. Deploy → 5. Verify
   ↓            ↓                     ↓               ↓            ↓
 staging    Local server         CI/CD checks   Production     Monitoring
 branch     (http://localhost)   passes         server         & alerts
```

### Step-by-Step Workflow

#### **1. Development (dev branch)**
- Create an ephemeral dev branch from `staging`
- Make changes and commit regularly on the dev branch
- Test changes in local development environment
- Do not merge to `staging` until the milestone is completed and verified locally

#### **2. Local Verification**
- **Required**: Test all changes locally before merging
- Run `./layoff-tracker` locally
- Verify: Dashboard loads, charts work, API responds
- Check: No console errors, proper data display
- Test: All user interactions work correctly

#### **3. Merge to Main**
- **Only after local verification and explicit approval**
- Merge dev → `staging` and wait for CI/CD checks to pass
- Merge `staging` → `main` only when tests pass and review is complete

#### **4. Production Deployment**
- Run `./deploy-local.sh` from main branch
- Monitor deployment logs for issues
- Verify health checks pass

#### **5. Post-Deployment Verification**
- Check production site loads correctly
- Verify all features work as expected
- Monitor error logs for 24 hours
- Be ready to rollback if issues appear

### Local Testing Requirements

#### **Must Verify Before Merging**
- [ ] Dashboard loads without errors
- [ ] All 4 charts display data correctly
- [ ] API endpoints return expected data
- [ ] Search and filtering work
- [ ] Mobile responsive design
- [ ] No console JavaScript errors
- [ ] Page load time acceptable (< 2 seconds)

#### **Local Development Setup**
```bash
# Start local server
go run main.go

# Or build and run
go build -o layoff-tracker .
./layoff-tracker
```

#### **Common Local Testing Issues**
- **Database not updated**: Run import process locally
- **Assets not loading**: Check static file paths
- **API errors**: Verify database schema matches code
- **Chart not loading**: Check browser console for JavaScript errors

## Pre-Deployment Checklist

### Code Quality
- [ ] All tests pass: `go test ./...`
- [ ] Code builds successfully: `go build`
- [ ] Linting passes (if configured)
- [ ] No TODO comments for production blockers

### Git Workflow
- [ ] All changes committed to `staging` branch
- [ ] `staging` and `main` branches are synchronized
- [ ] CI/CD pipeline passes for `staging` branch
- [ ] Manual review completed for `staging` → `main` merge

### Database Considerations
- [ ] Schema migrations tested on staging
- [ ] Backup strategy in place
- [ ] Data migration plan documented
- [ ] Rollback plan for schema changes

### Assets & Dependencies
- [ ] All templates updated: `templates/*.html`
- [ ] Static assets current: `static/*` files
- [ ] Environment variables documented
- [ ] External service credentials validated

## Production Deployment

### Automated CI Pipeline
1. **Automatic CI:** Push to `main` → GitHub Actions run tests, security scans, and build artifacts
2. **Manual CI:** Optional - trigger "Production CI" workflow for validation without push

### Local Deployment Process
1. **Prerequisites:**
   - GitHub CLI installed: `gh auth login`
   - SSH key for server access (default: `~/.ssh/github_actions_key`)
   - Application running on target server

2. **Configuration:**
   ```bash
   # Optional environment variables
   export SERVER_HOST="workforceloss.com"          # default: workforceloss.com
   export SSH_KEY_PATH="$HOME/.ssh/my_key"         # if different key location
   export GITHUB_REPO="sgarver/workforce-loss-tracker" # if different repo
   export DATABASE_PATH="/var/lib/layoff-tracker/layoff_tracker.db" # service override
   ```
   Note: do not use `/tmp` for production databases. `/tmp` can be cleaned on reboot.

3. **Execute Deployment:**
   ```bash
   ./deploy-local.sh
   ```

4. **What Gets Deployed:**
   - **Binary:** `layoff-tracker` (Go executable)
   - **Templates:** `templates/*.html` (runtime-loaded)
   - **Static Assets:** `static/*` (CSS, JS, images)
   - **Database:** Auto-migration on startup

5. **Deployment Process:**
   - Downloads latest successful CI artifact
   - Creates server backup of current binary
   - SCP binary + templates + static files to server
   - SSH executes: stop → backup → deploy → start
   - Health check with automatic rollback on failure

## Post-Deployment Verification

### Immediate Checks
- [ ] Health check passes: `curl https://workforceloss.com/ping`
- [ ] Main page loads: `curl https://workforceloss.com/ | grep -q "<!DOCTYPE html"`
- [ ] API endpoints respond: `curl https://workforceloss.com/api/stats | jq '.company_breakdown | length'`
- [ ] All expected assets load (check browser dev tools for 404s)

### Feature Verification
- [ ] Core functionality works (search, filtering, etc.)
- [ ] New features operational (charts, UI updates)
- [ ] Database queries successful (no SQL errors)
- [ ] External integrations working (if any)

### Performance Checks
- [ ] Page load times acceptable (< 3 seconds)
- [ ] API response times reasonable (< 1 second)
- [ ] Memory/CPU usage normal
- [ ] Error rates in logs acceptable

## Rollback Procedures

### Automatic Rollback
The deploy script automatically rolls back if health check fails within 30 seconds of startup.

### Manual Rollback (Server Side)
```bash
# SSH to production server
ssh linuxuser@workforceloss.com

# Navigate to app directory
cd /opt/layoff-tracker

# List available backups
ls -la layoff-tracker.backup.*

# Restore latest backup (replace TIMESTAMP)
cp layoff-tracker.backup.TIMESTAMP layoff-tracker

# Restart service
sudo systemctl restart layoff-tracker

# Verify restoration
curl http://localhost:8080/ping
```

### Emergency Rollback
If service won't start:
```bash
# Check service status
sudo systemctl status layoff-tracker

# View recent logs
sudo journalctl -u layoff-tracker -n 50

# Check for common issues:
# - Database corruption: Remove and recreate layoff_tracker.db
# - Missing files: Verify all templates/static files present
# - Port conflicts: Check if port 8080 is available
```

## Staging Deployment

### Process
1. Push feature branch → create PR to `staging`
2. Automated CI runs tests/security/build on `staging`
3. Manual review of changes and CI results
4. Merge `staging` → `main` for production deployment

### Staging Environment
- **URL:** https://staging.workforceloss.com (if configured)
- **Purpose:** Pre-production testing and validation
- **Data:** Copy of production data or anonymized subset

## Troubleshooting

### Common Deployment Issues

#### Missing Assets (Templates/CSS/JS)
**Symptoms:** Pages load but look broken, missing UI elements
**Cause:** Deployment script didn't copy frontend assets
**Fix:** Manually copy templates and static directories:
```bash
scp -r templates/ linuxuser@workforceloss.com:/opt/layoff-tracker/
scp -r static/ linuxuser@workforceloss.com:/opt/layoff-tracker/
sudo systemctl restart layoff-tracker
```

#### Database Schema Mismatch
**Symptoms:** Import fails with "no column named X" errors
**Cause:** Code expects different schema than production database
**Fix:** Run migrations or recreate database with correct schema

#### Service Won't Start
**Symptoms:** systemctl status shows failed state
**Check:**
```bash
sudo journalctl -u layoff-tracker -n 20
# Look for: database errors, missing files, port conflicts
```

#### API Returns Empty Data
**Symptoms:** Charts don't load, API returns `[]` or `{}`
**Cause:** Import failed or database is empty
**Check:**
```bash
# Check import logs
sudo journalctl -u layoff-tracker | grep -i import

# Check database
sqlite3 /opt/layoff-tracker/layoff_tracker.db "SELECT COUNT(*) FROM layoffs;"
```

### Performance Issues
- **Slow page loads:** Check database query performance
- **High memory usage:** Monitor for memory leaks
- **API timeouts:** Check database connection pool

## Monitoring & Maintenance

### Health Checks
- **Service:** `curl https://workforceloss.com/ping`
- **Database:** Check connection and query performance
- **External APIs:** Verify third-party service availability

### Logs
- **Application:** `sudo journalctl -u layoff-tracker`
- **System:** `/var/log/syslog` or `journalctl`
- **Access:** nginx/apache logs if using reverse proxy

### Backups
- **Automatic:** Daily database backups
- **Manual:** Before major changes
- **Retention:** Keep 7 daily, 4 weekly, 12 monthly backups

## Lessons Learned (Recent Deployment Issues)

### Issue: Missing Template Files
- **Problem:** Binary updated but templates not deployed
- **Impact:** UI showed old version with missing charts
- **Fix:** Updated `deploy-local.sh` to copy all assets
- **Prevention:** Always verify all asset types deployed

### Issue: Database Schema Drift
- **Problem:** Local dev schema differed from production
- **Impact:** Import failures and data inconsistencies
- **Fix:** Recreated production database with correct schema
- **Prevention:** Use migrations and test schema changes

### Issue: Incomplete Verification
- **Problem:** Only checked basic health, missed UI issues
- **Impact:** Deployed broken interface
- **Fix:** Added comprehensive post-deployment checklist
- **Prevention:** Always verify all user-facing functionality

### Best Practices Added
- [ ] **Asset Verification:** Check all file types deployed
- [ ] **Schema Sync:** Ensure database schemas match
- [ ] **UI Testing:** Verify all pages and interactions
- [ ] **Rollback Ready:** Test rollback procedures
- [ ] **Monitoring:** Set up alerts for critical metrics
