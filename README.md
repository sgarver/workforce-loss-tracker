# Layoff Tracker

A web application for tracking layoffs across industries using data from public WARN Act filings. Features include automated data import, web dashboard, filtering and search, layoff detail pages, CSV export, and notification system.

## Features

- **Automated Data Import**: Nightly import of WARN Act filings from all US states
- **Web Dashboard**: Overview with statistics, trends, and industry breakdowns
- **Layoff Tracker**: Browse layoffs with advanced filtering (industry, date range, employee count, search)
- **Layoff Management**: View detailed layoff information, add new layoff reports
- **Industry Overview**: Statistics and breakdowns by industry
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

### Layoff Management
- `GET /layoffs/:id` - Get detailed layoff information
- `GET /layoffs/new` - Form for reporting new layoffs
- `POST /layoffs` - Create new layoff report

### Web Interface
- `GET /` or `GET /dashboard` - Dashboard with statistics and trends
- `GET /tracker` - Layoff tracker with filtering and search
- `GET /industries` - Industry overview page
- `GET /export/csv` - Export filtered layoffs to CSV

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