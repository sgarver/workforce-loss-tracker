#!/bin/bash

# Pre-push checks for Workforce Loss Tracker
# Run this before pushing to ensure code quality

echo "🔍 Running pre-push checks..."

# Check if in git repo
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "❌ Not in a git repository"
    exit 1
fi

# Check for unstaged changes
if ! git diff --quiet; then
    echo "⚠️  You have unstaged changes. Consider staging them or stashing."
fi

# Check for uncommitted changes
if ! git diff --cached --quiet; then
    echo "⚠️  You have staged but uncommitted changes."
fi

# Build check
echo "🔨 Checking Go build..."
if ! go build -o /tmp/layoff-tracker-test ./; then
    echo "❌ Go build failed"
    exit 1
fi
echo "✅ Go build successful"

# Test check
echo "🧪 Running Go tests..."
if ! go test ./... -v; then
    echo "❌ Go tests failed"
    exit 1
fi
echo "✅ Go tests passed"

# YAML check (if yamllint available)
if command -v yamllint > /dev/null 2>&1; then
    echo "📄 Checking YAML files..."
    if ! yamllint .github/workflows/*.yml; then
        echo "❌ YAML linting failed"
        exit 1
    fi
    echo "✅ YAML linting passed"
else
    echo "⚠️  yamllint not available - install with: pip install yamllint"
fi

echo "🎉 All pre-push checks passed! Safe to push."
exit 0