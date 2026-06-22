#!/bin/bash
# Security vulnerability scan using Trivy
set -e

echo "=== CloudFlow Security Scan ==="

# Check if trivy is installed
if ! command -v trivy &> /dev/null; then
    echo "Installing Trivy..."
    curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin
fi

# Scan Go dependencies for known vulnerabilities
echo "Scanning Go dependencies..."
trivy fs --scanners vuln,secret,misconfig --severity HIGH,CRITICAL .

# Scan Docker images if built
for img in alert-engine data-plane query-service control-plane tenant-service auth-service; do
    if docker images "$img" | grep -q "$img"; then
        echo "Scanning Docker image: $img"
        trivy image --severity HIGH,CRITICAL "$img"
    fi
done

echo "=== Security Scan Complete ==="
