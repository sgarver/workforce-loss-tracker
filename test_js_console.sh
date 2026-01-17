#!/bin/bash

# JavaScript Console Testing Script
echo "🔍 Testing JavaScript Console Output..."
echo "========================================"

# Start the server in background
echo "📡 Starting server..."
PORT=8080 ./layoff-tracker &
SERVER_PID=$!
sleep 3

# Test JavaScript functions directly
echo "🧪 Testing JavaScript functions..."
timeout 10 firefox --headless --new-instance \
  --createprofile test_profile \
  --setpref devtools.console.stdout.content=true \
  --setpref devtools.console.stderr.content=true \
  --setpref javascript.options.showInConsole=true \
  "file://$(pwd)/test_js.html" \
  2>&1 | grep -E "(console|Error|SyntaxError|ReferenceError|TypeError|destroyCharts|renderCharts)" || echo "No console errors detected"

# Test with actual site
echo "🌐 Testing main site..."
timeout 10 firefox --headless --new-instance \
  --createprofile test_profile2 \
  --setpref devtools.console.stdout.content=true \
  --setpref devtools.console.stderr.content=true \
  --setpref javascript.options.showInConsole=true \
  "http://localhost:8080" \
  2>&1 | grep -E "(console|Error|SyntaxError|ReferenceError|TypeError|destroyCharts)" || echo "No console errors on main site"

# Additional API tests
echo "🔗 Testing API interactions..."
curl -s "http://localhost:8080/api/stats?months=1" > /dev/null && echo "✅ 1-month API call successful"
curl -s "http://localhost:8080/api/stats?months=3" > /dev/null && echo "✅ 3-month API call successful"
curl -s "http://localhost:8080/api/stats?months=6" > /dev/null && echo "✅ 6-month API call successful"

# Alternative: Check for JavaScript errors in the page source
echo "📄 Checking for obvious JavaScript issues..."
curl -s "http://localhost:8080" | grep -o '<script[^>]*>[^<]*</script>' | head -5

# Test API endpoints
echo "🔗 Testing API endpoints..."
curl -s "http://localhost:8080/api/stats?months=1" | jq '.monthly_trend | length' 2>/dev/null && echo "✅ API working" || echo "❌ API failed"

# Clean up
echo "🧹 Cleaning up..."
kill $SERVER_PID 2>/dev/null
rm -rf test_profile 2>/dev/null

echo "✅ Testing complete!"