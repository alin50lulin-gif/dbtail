#!/bin/sh
set -e

mkdir -p /etc/dbtail/states
chmod 0755 /etc/dbtail/states

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl enable dbtail.service >/dev/null 2>&1 || true
fi

exit 0
