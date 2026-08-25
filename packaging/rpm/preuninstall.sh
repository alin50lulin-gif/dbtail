#!/bin/sh
set -e

# RPM passes 0 for final removal and 1 for upgrade.
if [ "${1:-0}" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now dbtail.service >/dev/null 2>&1 || true
fi

exit 0
