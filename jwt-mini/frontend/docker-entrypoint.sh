#!/bin/sh
set -e

API_BASE="${API_BASE:-http://localhost:8080}"

cat > /usr/share/nginx/html/config.js <<EOF
window.APP_CONFIG = {
  API_BASE: "${API_BASE}",
};
EOF

echo "frontend API_BASE=${API_BASE}"
exec nginx -g "daemon off;"
