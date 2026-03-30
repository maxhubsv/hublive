#!/bin/bash
set -euo pipefail

echo "=== Checking for circular imports ==="
go build ./... 2>&1 | grep -i "import cycle" && {
    echo "FAIL: Circular import detected"
    exit 1
} || echo "PASS: No circular imports"

echo ""
echo "=== Checking dependency rule violations ==="

VIOLATIONS=""

# Domain layer must NOT import infrastructure (excluding test files)
if [ -d "pkg/domain" ]; then
    for pkg in store transport routing api service; do
        V=$(grep -rn "\"github.com/hublive/hublive-server/pkg/$pkg" pkg/domain/ --include="*.go" 2>/dev/null | grep -v "_test.go" || true)
        VIOLATIONS+="$V"
    done
fi

if [ -n "$VIOLATIONS" ]; then
    echo "FAIL: Domain layer importing infrastructure:"
    echo "$VIOLATIONS"
    exit 1
fi
echo "PASS: Domain layer imports clean (only rtc/types allowed)"

# Transport layer should NOT import service/
echo ""
if [ -d "pkg/transport" ]; then
    V=$(grep -rn '"github.com/hublive/hublive-server/pkg/service"' pkg/transport/ 2>/dev/null || true)
    if [ -n "$V" ]; then
        echo "FAIL: Transport layer importing service/:"
        echo "$V"
        exit 1
    fi
    echo "PASS: Transport layer independent from service/"
fi

# API layer should NOT import service/
if [ -d "pkg/api" ]; then
    V=$(grep -rn '"github.com/hublive/hublive-server/pkg/service"' pkg/api/ 2>/dev/null || true)
    if [ -n "$V" ]; then
        echo "FAIL: API layer importing service/:"
        echo "$V"
        exit 1
    fi
    echo "PASS: API layer independent from service/"
fi

echo ""
echo "=== Layer dependency matrix ==="
for dir in domain store transport api; do
    if [ -d "pkg/$dir" ]; then
        echo "$dir/ imports:"
        grep -roh '"github.com/hublive/hublive-server/pkg/[^"]*"' "pkg/$dir/" 2>/dev/null | sort -u | sed 's/.*pkg\//  /' || echo "  (none)"
        echo ""
    fi
done

echo "=== All checks passed ==="
