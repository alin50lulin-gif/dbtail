CREATE TABLE IF NOT EXISTS dbtail.dw_pg_sql_logs_{ip}_{port}
(
    `_time` DateTime,
    `_date` Date DEFAULT toDate(`_time`),
    `_ms` UInt32,
    `_insert_time` DateTime64(3) DEFAULT now64(3),

    `application` LowCardinality(String),
    `user` LowCardinality(String),
    `database` LowCardinality(String),
    `host` String,
    `host_port` String,

    `pid` UInt32,
    `command_tag` LowCardinality(String),
    `sql_state` LowCardinality(String),
    `session_id` String,
    `session_line_number` UInt32,
    `session_start` String,
    `virtual_transaction_id` String,
    `transaction_id` String,

    `duration` Float64,
    `query` String,
    `query_text` String ALIAS query,
    `normalized_query` String,
    `tables` String,
    `comments` String,

    `query_id` String DEFAULT '',
    `plan` String DEFAULT '',
    `event_time` DateTime ALIAS `_time`,

    `ref_tables` Array(String)
        MATERIALIZED arrayDistinct(
            arrayMap(
                x -> concat(x[2], '.', x[1]),
                extractAllGroups(
                    plan,
                    '"Relation Name"[[:space:]]*:[[:space:]]*"([^"]+)"[[:space:]]*,[[:space:]]*"Schema"[[:space:]]*:[[:space:]]*"([^"]+)"'
                )
            )
        ),

    `plan_node_types` Array(String)
        MATERIALIZED extractAll(
            plan,
            '"Node Type"[[:space:]]*:[[:space:]]*"([^"]+)"'
        ),

    INDEX idx_qid query_id TYPE minmax GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY `_date`
ORDER BY (`_time`, `_ms`, `database`, `session_id`)
-- Optional retention policy: uncomment to delete PostgreSQL log rows after 1 month.
-- TTL `_time` + INTERVAL 1 MONTH DELETE
SETTINGS index_granularity = 8192;
