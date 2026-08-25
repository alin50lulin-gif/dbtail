# DBtail

[English](#english) | [中文](#中文)

## English

DBtail parses database and web-server log files and sends structured events to ClickHouse. Contributions, secondary development, new log-format adapters, issues, and pull requests are welcome.

### Project background

DBtail is a maintained and extended fork of [Altinity/clicktail](https://github.com/Altinity/clicktail), which was originally developed from [honeycombio/honeytail](https://github.com/honeycombio/honeytail):

```text
honeytail -> clicktail -> dbtail
```

The current development effort focuses on three areas:

1. PostgreSQL log parsing, `auto_explain` plans, and timestamp-based rotation.
2. MySQL, MariaDB, and Percona slow-log parsing and numbered rotation.
3. Shared tailing, state files, file-handle lifecycle, systemd, and RPM packaging.

The existing MongoDB, ArangoDB, Nginx, Regex, and MySQL Audit parsers remain available. They were not part of the production validation performed on August 24–25, 2026, so validate them against your own log formats before production use.

### August 24–25, 2026 update

#### PostgreSQL

Fixed and enhanced:

- Supports modern PostgreSQL log prefixes, including `%Q` Query Identifier, hexadecimal session IDs, and application names containing spaces.
- Parses multiline `auto_explain` JSON plans and extracts SQL text, Query ID, referenced tables, and plan node types.
- Supports timestamp-based files such as `postgresql-%Y-%m-%d_%H%M%S.log`.
- Reads historical files while immediately discovering and reading the latest file, preventing backlog from blocking real-time collection.
- Fixes state-file resume, completed offsets being reset to zero, and stale state-file cleanup.
- Releases file handles after a logfile reaches EOF, so deleted logs no longer continue occupying disk space.
- Adds `IgnoreQueryRegex` and a fast path for filtering queries such as `SELECT 1`, reducing JSON parsing, network, and ClickHouse storage overhead.
- Updates the ClickHouse schema with plan fields and compatibility aliases such as `query_text` and `event_time`.
- Provides an optional one-month TTL example.

Validated with production-style logs:

- Automatic discovery of newly rotated files.
- Concurrent reading of historical and current files.
- Resume from the saved inode and offset after restarting DBtail.
- Stable final offset equal to the completed file size.
- File-handle release after EOF.
- Runtime cleanup of state files whose source logs were removed.
- Successful parsing and insertion of multiline JSON plans.
- `SELECT 1` filtering while other SQL continues to reach ClickHouse.

#### MySQL, MariaDB, and Percona

Fixed and enhanced:

- Extends slow-log parsing for common MySQL, MariaDB, and Percona fields.
- Uses the ClickHouse table naming convention `mysql_slow_log_{ip}_{port}`.
- Supports Percona numbered rotation such as `slow.log.000001` and dynamically discovers later sequence files.
- Provides an optional six-month TTL example.

Validated with Percona Server 8.0.40-31:

- Slow-log events are parsed and inserted into ClickHouse.
- Numbered files rotate continuously; the highest sequence is the newest active file.
- Restart resumes each file from its own state file.
- Completed historical files release their handles.

Use a filesystem glob for Percona numbered rotation:

```ini
LogFiles = /mysql/data3307/log/slow.log*
StateFile = /etc/dbtail/states/
```

Do not use regex-style escaping such as `slow\.log\*`, and do not monitor only the old `slow.log`. Standard MySQL, external `logrotate`, and Percona internal rotation can behave differently; verify the active path with `@@slow_query_log_file` and by observing which file continues to grow.

#### Shared tailing and deployment

- Dynamically discovers new files matching a configured glob.
- Tails multiple files concurrently.
- Creates the state directory and maintains per-file resume offsets.
- Cleans state files for removed logs.
- Fixes races between file closing and offset persistence.
- Rejects common configuration mistakes such as `StateFile = .../*`, escaped glob wildcards, and Markdown-formatted `APIHost` values.
- Provides a statically linked Linux amd64 binary and a systemd unit.
- Provides RPM packaging with the binary, configuration, systemd unit, ClickHouse schemas, state directory, and installation guide.
- Protects an existing `/etc/dbtail/dbtail.conf` during upgrades using RPM `noreplace` behavior.

### Supported parsers

- [ArangoDB](parsers/arangodb/)
- [MongoDB](parsers/mongodb/)
- [MySQL](parsers/mysql/)
- [PostgreSQL](parsers/postgresql/)
- [Nginx](parsers/nginx/)
- [Regex](parsers/regex/)
- [MySQL Audit](parsers/mysqlaudit/)
- JSON and key-value logs

### RPM installation

DBtail currently ships as an RPM for Linux x86_64.

Install:

```bash
rpm -ivh dbtail-1.0.0-1.x86_64.rpm
```

Upgrade:

```bash
rpm -Uvh dbtail-1.0.0-1.x86_64.rpm
```

Installed files:

- `/usr/bin/dbtail`
- `/etc/dbtail/dbtail.conf`
- `/etc/dbtail/states/`
- `/usr/lib/systemd/system/dbtail.service`
- `/usr/share/dbtail/schema/`
- `/usr/share/doc/dbtail/README.md`

The package enables the service for automatic startup but does not start it with the placeholder configuration. Configure DBtail and create the ClickHouse table first, then run:

```bash
systemctl start dbtail
systemctl status dbtail
journalctl -u dbtail -f
```

### Configuration examples

MySQL/Percona:

```ini
[Application Options]
APIHost = http://user:password@clickhouse-host:8123/
NumSenders = 2
BatchFrequencyMs = 10000
BatchSize = 1000

[Required Options]
ParserName = mysql
LogFiles = /mysql/data3307/log/slow.log*
Dataset = dbtail.mysql_slow_log_10_10_184_213_3309

[Tail Options]
ReadFrom = last
Stop = false
StateFile = /etc/dbtail/states/
```

PostgreSQL:

```ini
[Application Options]
APIHost = http://user:password@clickhouse-host:8123/
NumSenders = 4
BatchFrequencyMs = 10000
BatchSize = 1000

[Required Options]
ParserName = postgresql
LogFiles = /postgresql/data5432/log/postgresql-*.log
Dataset = dbtail.dw_pg_sql_logs_10_10_24_90_5432

[PostgreSQL Parser Options]
LogLinePrefix = %m [%p] [%Q]:[%c]:[%l] %u@%d [%r]: [%a/%i] [%v/%x]
IgnoreQueryRegex = (?i)^\s*select\s+1\s*;?\s*$

[Tail Options]
ReadFrom = last
Stop = false
StateFile = /etc/dbtail/states/
```

`LogFiles` uses filesystem glob syntax, not regular expressions. `StateFile` is a directory and must not end in `*`.

### ClickHouse schema

RPM installs the schemas under `/usr/share/dbtail/schema/`:

```bash
clickhouse-client --multiline < /usr/share/dbtail/schema/db.sql
clickhouse-client --multiline < /usr/share/dbtail/schema/mysql.sql
# or
clickhouse-client --multiline < /usr/share/dbtail/schema/postgresql.sql
```

Review the table name and optional TTL statement before executing a schema.

---

## 中文

DBtail 用于解析数据库和 Web 服务器日志，并将结构化事件写入 ClickHouse。欢迎大家基于本项目进行二次开发、适配新的日志格式，以及提交 Issue 和 Pull Request。

### 项目背景

DBtail 是 [Altinity/clicktail](https://github.com/Altinity/clicktail) 的持续维护和增强版本，而 clicktail 最初基于 [honeycombio/honeytail](https://github.com/honeycombio/honeytail) 开发：

```text
honeytail -> clicktail -> dbtail
```

当前阶段的专项优化主要集中在三个部分：

1. PostgreSQL 日志解析、`auto_explain` 执行计划和时间戳轮转。
2. MySQL、MariaDB、Percona 慢日志解析和编号轮转。
3. 公共 tail、statefile、文件句柄、systemd 和 RPM 部署能力。

MongoDB、ArangoDB、Nginx、Regex、MySQL Audit 等原有解析器仍然保留并可继续使用，但不属于 2026 年 8 月 24–25 日这轮专项生产验证范围，正式使用前应根据实际日志格式单独验证。

### 2026 年 8 月 24–25 日重要更新

#### PostgreSQL 模块

已修复和增强：

- 兼容新版 PostgreSQL 日志前缀，包括 `%Q` Query Identifier、十六进制 session ID，以及包含空格的 application name。
- 支持 `auto_explain` 多行 JSON 执行计划，提取 SQL、Query ID、关联表和执行计划节点类型。
- 支持 `postgresql-%Y-%m-%d_%H%M%S.log` 时间戳轮转。
- 历史文件继续读取的同时立即发现并读取最新日志，避免历史积压阻塞实时日志。
- 修复 statefile 断点续读、完成 offset 被错误归零和过期 statefile 清理问题。
- 文件读取到 EOF 后释放句柄，避免已删除日志继续占用磁盘空间。
- 增加 `IgnoreQueryRegex` 和快速过滤路径，可过滤 `SELECT 1`，减少 JSON 解析、网络发送和 ClickHouse 存储开销。
- 更新 ClickHouse 表结构，增加执行计划字段及 `query_text`、`event_time` 等兼容别名。
- 提供可选的一个月 TTL 示例。

实际日志环境已验证：

- 自动发现时间戳轮转产生的新文件。
- 历史文件和当前文件并行读取。
- 重启后从 statefile 保存的 inode 和 offset 继续读取。
- 文件完成后的 offset 等于文件大小且不会归零。
- 文件读取完成后正常释放句柄。
- 日志被删除后，在运行期间自动清理对应 statefile，无需重启。
- 多行 JSON plan 正常解析并写入 ClickHouse。
- `SELECT 1` 停止入库，其他 SQL 继续写入。

#### MySQL、MariaDB 和 Percona 模块

已修复和增强：

- 扩展慢日志解析，兼容常见 MySQL、MariaDB 和 Percona 字段。
- ClickHouse 表名统一为 `mysql_slow_log_{ip}_{port}`。
- 支持 `slow.log.000001`、`slow.log.000002` 等 Percona 编号轮转，并动态发现后续编号文件。
- 提供可选的六个月 TTL 示例。

已在 Percona Server 8.0.40-31 环境验证：

- 慢日志正常解析并写入 ClickHouse。
- 编号文件可以连续轮转，编号最大的文件是最新活动日志。
- 重启后根据每个文件对应的 statefile 断点续读。
- 历史文件读取完成后释放文件句柄。

Percona 编号轮转配置：

```ini
LogFiles = /mysql/data3307/log/slow.log*
StateFile = /etc/dbtail/states/
```

不要写成正则形式的 `slow\.log\*`，也不要只监听旧的 `slow.log`。标准 MySQL、外部 `logrotate` 与 Percona 内置轮转行为可能不同，应通过 `@@slow_query_log_file` 和持续增长的实际文件确认当前活动路径。

#### 公共模块与部署

- 动态发现 glob 新匹配的日志文件。
- 多文件并行 tail，避免历史大文件阻塞最新日志。
- 自动创建 state 目录，保存每个文件的断点位置，并清理已删除日志对应的 statefile。
- 修复文件关闭与 offset 保存之间的并发问题。
- 检查 `StateFile = .../*`、转义错误的 glob 和 Markdown 格式 `APIHost` 等常见配置错误。
- 提供 Linux amd64 静态二进制和 systemd unit。
- RPM 包包含二进制、配置文件、systemd unit、ClickHouse schema、state 目录和安装说明。
- RPM 升级使用 `noreplace` 机制保护已有 `/etc/dbtail/dbtail.conf`。

### 支持的解析器

- [ArangoDB](parsers/arangodb/)
- [MongoDB](parsers/mongodb/)
- [MySQL](parsers/mysql/)
- [PostgreSQL](parsers/postgresql/)
- [Nginx](parsers/nginx/)
- [Regex](parsers/regex/)
- [MySQL Audit](parsers/mysqlaudit/)
- JSON 和 key-value 日志

### RPM 安装

DBtail 当前提供 Linux x86_64 RPM 包。

首次安装：

```bash
rpm -ivh dbtail-1.0.0-1.x86_64.rpm
```

升级：

```bash
rpm -Uvh dbtail-1.0.0-1.x86_64.rpm
```

安装内容：

- `/usr/bin/dbtail`
- `/etc/dbtail/dbtail.conf`
- `/etc/dbtail/states/`
- `/usr/lib/systemd/system/dbtail.service`
- `/usr/share/dbtail/schema/`
- `/usr/share/doc/dbtail/README.md`

RPM 会将服务设置为开机自启，但不会使用占位配置立即启动。完成配置并创建 ClickHouse 表后执行：

```bash
systemctl start dbtail
systemctl status dbtail
journalctl -u dbtail -f
```

### 配置示例

MySQL/Percona：

```ini
[Application Options]
APIHost = http://user:password@clickhouse-host:8123/
NumSenders = 2
BatchFrequencyMs = 10000
BatchSize = 1000

[Required Options]
ParserName = mysql
LogFiles = /mysql/data3307/log/slow.log*
Dataset = dbtail.mysql_slow_log_10_10_184_213_3309

[Tail Options]
ReadFrom = last
Stop = false
StateFile = /etc/dbtail/states/
```

PostgreSQL：

```ini
[Application Options]
APIHost = http://user:password@clickhouse-host:8123/
NumSenders = 4
BatchFrequencyMs = 10000
BatchSize = 1000

[Required Options]
ParserName = postgresql
LogFiles = /postgresql/data5432/log/postgresql-*.log
Dataset = dbtail.dw_pg_sql_logs_10_10_24_90_5432

[PostgreSQL Parser Options]
LogLinePrefix = %m [%p] [%Q]:[%c]:[%l] %u@%d [%r]: [%a/%i] [%v/%x]
IgnoreQueryRegex = (?i)^\s*select\s+1\s*;?\s*$

[Tail Options]
ReadFrom = last
Stop = false
StateFile = /etc/dbtail/states/
```

`LogFiles` 使用文件系统 glob，不是正则表达式；`StateFile` 是目录，末尾不能添加 `*`。

### ClickHouse 建表

RPM 将建表文件安装到 `/usr/share/dbtail/schema/`：

```bash
clickhouse-client --multiline < /usr/share/dbtail/schema/db.sql
clickhouse-client --multiline < /usr/share/dbtail/schema/mysql.sql
# 或者
clickhouse-client --multiline < /usr/share/dbtail/schema/postgresql.sql
```

执行前请根据实际环境检查表名和可选 TTL 语句。
