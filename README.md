# Workforce Loss Tracker

[![CI](https://github.com/sgarver/workforce-loss-tracker/actions/workflows/production-deploy.yml/badge.svg?branch=main)](https://github.com/sgarver/workforce-loss-tracker/actions/workflows/production-deploy.yml)
[![Staging CI](https://github.com/sgarver/workforce-loss-tracker/actions/workflows/staging-ci.yml/badge.svg?branch=staging)](https://github.com/sgarver/workforce-loss-tracker/actions/workflows/staging-ci.yml)

A web application for tracking workforce reductions across industries using data from public WARN Act filings. Features include automated data import, web dashboard, filtering and search, layoff detail pages, CSV export, and notification system.

## Features

- **Automated Data Import**: Nightly import of WARN Act filings from all US states
- **Web Dashboard**: Overview with statistics, trends, and industry breakdowns
- **Workforce Loss Tracker**: Browse workforce losses with advanced filtering (industry, date range, employee count, search)
- **Workforce Loss Management**: View detailed workforce loss information, add new workforce loss reports
- **Industry Overview**: Statistics and breakdowns by industry

## Deployment

See [DEPLOY.md](DEPLOY.md) for deployment and rollback procedures.
- **CSV Export**: Export filtered layoff data to CSV
- **API**: RESTful API for statistics and layoff data
- **Notifications**: Email notifications for import status and failures

## Data Sources

### Current Data Sources
- **WARN Database**: Comprehensive database of WARN Act notices from all US states (primary source)
  - Source: https://layoffdata.com/data/
  - Contains individual company layoff records with employee counts, dates, and locations
  - Import endpoint: `POST /import/warn`

### Deprecated Data Sources (No Longer Available)
- ~~GitHub Open Data (realspinn/layoffs_data_cleaning_project)~~ - Repository no longer maintained
- ~~USLayoffs.org API~~ - Service discontinued

## API Endpoints

### Data Import
- `POST /import/warn` - Import latest data from WARN Database
- `POST /import/revelio` - Check aggregated data from Revelio Labs (for reference)

### Layoff Data
- `GET /api/stats` - Overall statistics (with optional months parameter)
- `GET /api/layoffs` - List of layoffs with filtering and pagination
- `GET /api/industries` - Available industries
- `GET /api/sponsored` - Sponsored listings
- `GET /api/current-layoffs` - Recent layoffs (last 30 days)

### Workforce Loss Management
- `GET /layoffs/:id` - Get detailed workforce loss information
- `GET /layoffs/new` - Form for reporting new workforce losses
- `POST /layoffs` - Create new workforce loss report

### Web Interface
- `GET /` or `GET /dashboard` - Dashboard with statistics and trends
- `GET /tracker` - Workforce loss tracker with filtering and search
- `GET /industries` - Industry overview page
- `GET /export/csv` - Export filtered workforce losses to CSV

## Dependencies

- Go 1.24+
- Echo web framework
- SQLite3 database

## Running the Application

### Standard Development Mode

```bash
go run main.go
```

The server will start on port 8080 (configurable via PORT environment variable). Static files are served from the `static/` directory, and HTML templates from `templates/`.

### Systemd Service for Development (Recommended)

For production-like development with persistent logging and automatic restarts, use the systemd service:

**1. Install the Service**
```bash
sudo cp layoff-tracker-dev.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable layoff-tracker-dev
```

**2. Start the Service**
```bash
sudo systemctl start layoff-tracker-dev
```

**3. View Logs**
```bash
sudo journalctl -u layoff-tracker-dev -f
```

**4. Service Management**
```bash
# Restart after code changes
sudo systemctl restart layoff-tracker-dev

# Stop service
sudo systemctl stop layoff-tracker-dev

# Check status
sudo systemctl status layoff-tracker-dev
```

**Benefits:**
- Persistent logs that survive terminal sessions
- Automatic restarts on crashes
- Production-like process management
- Easy debugging with `journalctl`

## Linting and Formatting

Run linting and formatting checks before committing:

```bash
go vet ./...
go fmt ./...
```

## Database

Uses SQLite (`layoff_tracker.db`) with automatic migrations. Tables include:

- `industries` - Industry categories
- `companies` - Company information with industry links
- `layoffs` - Layoff records with employee counts, dates, and sources
- `sponsored_listings` - Promotional listings for companies
- `import_history` - Tracking of data import operations

## Importing Data

The application includes automated nightly imports. To manually import data:

```bash
curl -X POST http://localhost:8080/import/warn
```

This will download and process the latest WARN Act filings from all US states.

For production deployment, configure email notifications for import status updates.
# Test CI/CD pipeline
