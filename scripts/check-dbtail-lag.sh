#!/usr/bin/env bash

# Check whether DBtail-managed ClickHouse tables have received recent events.
# Exit codes: 0 = healthy, 1 = stale/empty table, 2 = configuration/query error.
set -uo pipefail

CH_CLIENT=${CH_CLIENT:-clickhouse-client}
CH_HOST=${CH_HOST:-127.0.0.1}
CH_PORT=${CH_PORT:-9000}
CH_DATABASE=${CH_DATABASE:-dbtail}
# Replace these placeholders with the actual ClickHouse monitoring credentials,
# preferably by setting CH_USER and CH_PASSWORD in the local cron environment.
CH_USER=${CH_USER:-dbtail}
CH_PASSWORD=${CH_PASSWORD:-password}
LAG_THRESHOLD_SECONDS=${LAG_THRESHOLD_SECONDS:-600}
TABLE_REGEX=${TABLE_REGEX:-'^(dw_pg_sql_logs_|mysql_slow_log_)[A-Za-z0-9_]+$'}
ALERT_COMMAND=${ALERT_COMMAND:-}
SYSLOG_ALERT=${SYSLOG_ALERT:-true}
# Replace REPLACE_ME with the access_token URL of a DingTalk custom robot.
# Leave the placeholder unchanged to disable DingTalk notifications.
DINGTALK_WEBHOOK=${DINGTALK_WEBHOOK:-'https://oapi.dingtalk.com/robot/send?access_token=REPLACE_ME'}
DINGTALK_KEYWORD=${DINGTALK_KEYWORD:-DBtail}

if ! [[ "$LAG_THRESHOLD_SECONDS" =~ ^[0-9]+$ ]] || [ "$LAG_THRESHOLD_SECONDS" -eq 0 ]; then
    echo "ERROR: LAG_THRESHOLD_SECONDS must be a positive integer" >&2
    exit 2
fi

if ! [[ "$CH_DATABASE" =~ ^[A-Za-z0-9_]+$ ]]; then
    echo "ERROR: CH_DATABASE contains unsupported characters: $CH_DATABASE" >&2
    exit 2
fi

if ! command -v "$CH_CLIENT" >/dev/null 2>&1; then
    echo "ERROR: ClickHouse client not found: $CH_CLIENT" >&2
    exit 2
fi

client_args=(
    --host "$CH_HOST"
    --port "$CH_PORT"
    --user "$CH_USER"
    --password "$CH_PASSWORD"
    --database "$CH_DATABASE"
    --format TSVRaw
)

run_query() {
    "$CH_CLIENT" "${client_args[@]}" --query "$1"
}

emit_alert() {
    local message=$1
    echo "ALERT: $message" >&2
    if [ "$SYSLOG_ALERT" = "true" ] && command -v logger >/dev/null 2>&1; then
        logger -t dbtail-monitor -- "$message"
    fi
}

json_escape() {
    local value=$1
    value=${value//\\/\\\\}
    value=${value//\"/\\\"}
    value=${value//$'\n'/\\n}
    value=${value//$'\r'/\\r}
    printf '%s' "$value"
}

send_dingtalk_alert() {
    local message=$1
    local content payload response

    if [ -z "$DINGTALK_WEBHOOK" ] || [[ "$DINGTALK_WEBHOOK" == *REPLACE_ME* ]]; then
        return 0
    fi
    if ! command -v curl >/dev/null 2>&1; then
        emit_alert "cannot send DingTalk alert: curl is not installed"
        return 1
    fi

    content=$(json_escape "[$DINGTALK_KEYWORD] DBtail ingestion alert
$message")
    payload="{\"msgtype\":\"text\",\"text\":{\"content\":\"$content\"},\"at\":{\"isAtAll\":false}}"
    if ! response=$(curl --silent --show-error --fail \
        --connect-timeout 5 --max-time 10 \
        --header 'Content-Type: application/json' \
        --data "$payload" \
        "$DINGTALK_WEBHOOK" 2>&1); then
        emit_alert "failed to send DingTalk alert: $response"
        return 1
    fi
    if ! grep -Eq '"errcode"[[:space:]]*:[[:space:]]*0' <<< "$response"; then
        emit_alert "DingTalk rejected alert: $response"
        return 1
    fi
    return 0
}

dispatch_alert() {
    local message=$1
    local delivery_failed=0

    emit_alert "$message"
    if [ -n "$ALERT_COMMAND" ]; then
        if [ ! -x "$ALERT_COMMAND" ]; then
            emit_alert "ALERT_COMMAND is not executable: $ALERT_COMMAND"
            delivery_failed=1
        elif ! "$ALERT_COMMAND" "$message"; then
            emit_alert "ALERT_COMMAND failed: $ALERT_COMMAND"
            delivery_failed=1
        fi
    fi
    if ! send_dingtalk_alert "$message"; then
        delivery_failed=1
    fi
    return "$delivery_failed"
}

table_query="SELECT name
FROM system.tables
WHERE database = {database:String}
  AND (startsWith(name, 'dw_pg_sql_logs_') OR startsWith(name, 'mysql_slow_log_'))
ORDER BY name"

if ! tables=$("$CH_CLIENT" "${client_args[@]}" \
    --param_database "$CH_DATABASE" --query "$table_query" 2>&1); then
    dispatch_alert "cannot query ClickHouse tables: $tables" || true
    exit 2
fi

if [ -z "$tables" ]; then
    dispatch_alert "no DBtail tables found in database $CH_DATABASE" || true
    exit 2
fi

alerts=()
checked=0
query_errors=0

while IFS= read -r table; do
    [ -n "$table" ] || continue
    if ! [[ "$table" =~ $TABLE_REGEX ]]; then
        continue
    fi

    checked=$((checked + 1))
    status_query="SELECT
        formatDateTime(max(\`_time\`), '%F %T'),
        greatest(dateDiff('second', max(\`_time\`), now()), 0),
        count()
    FROM \`$CH_DATABASE\`.\`$table\`"

    if ! result=$(run_query "$status_query" 2>&1); then
        alerts+=("$CH_DATABASE.$table query failed: $result")
        query_errors=$((query_errors + 1))
        continue
    fi

    IFS=$'\t' read -r max_time lag_seconds row_count <<< "$result"
    if [ "${row_count:-0}" -eq 0 ]; then
        alerts+=("$CH_DATABASE.$table is empty")
    elif ! [[ "${lag_seconds:-}" =~ ^[0-9]+$ ]]; then
        alerts+=("$CH_DATABASE.$table returned invalid lag: $result")
        query_errors=$((query_errors + 1))
    elif [ "$lag_seconds" -gt "$LAG_THRESHOLD_SECONDS" ]; then
        alerts+=("$CH_DATABASE.$table is stale: max(_time)=$max_time, lag=${lag_seconds}s, threshold=${LAG_THRESHOLD_SECONDS}s")
    else
        printf 'OK: %s.%s max(_time)=%s lag=%ss rows=%s\n' \
            "$CH_DATABASE" "$table" "$max_time" "$lag_seconds" "$row_count"
    fi
done <<< "$tables"

if [ "$checked" -eq 0 ]; then
    dispatch_alert "no table matched TABLE_REGEX=$TABLE_REGEX in database $CH_DATABASE" || true
    exit 2
fi

if [ "${#alerts[@]}" -gt 0 ]; then
    summary=$(printf '%s\n' "${alerts[@]}")
    if ! dispatch_alert "$summary"; then
        exit 2
    fi
    if [ "$query_errors" -gt 0 ]; then
        exit 2
    fi
    exit 1
fi

echo "All $checked DBtail tables are within ${LAG_THRESHOLD_SECONDS}s."
exit 0
