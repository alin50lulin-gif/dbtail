# DBtail RPM installation

This package installs DBtail for Linux x86_64.

## Installed files

- `/usr/bin/dbtail`: statically linked DBtail executable
- `/etc/dbtail/dbtail.conf`: main configuration (`%config(noreplace)` on upgrade)
- `/etc/dbtail/states/`: persistent per-log offset state files
- `/usr/lib/systemd/system/dbtail.service`: systemd service unit
- `/usr/share/dbtail/schema/`: ClickHouse table schemas
- `/usr/share/doc/dbtail/README.md`: this document

## Installation

```bash
dnf install ./dbtail-1.0.0-1.x86_64.rpm
```

The package enables `dbtail.service`, but does not start it because the shipped
configuration contains placeholders. Edit `/etc/dbtail/dbtail.conf`, create the
matching ClickHouse table from `/usr/share/dbtail/schema/`, and then start it:

```bash
systemctl start dbtail
systemctl status dbtail
journalctl -u dbtail -f
```

## MySQL/Percona numbered slow-log rotation

```ini
[Required Options]
ParserName = mysql
LogFiles = /mysql/data3307/log/slow.log*
Dataset = dbtail.mysql_slow_log_10_10_184_213_3309

[Tail Options]
ReadFrom = last
Stop = false
StateFile = /etc/dbtail/states/
```

`LogFiles` uses filesystem glob syntax, not regular-expression syntax. Do not
write `slow\.log\*`. The state path is a directory and must not end in `*`.

## PostgreSQL timestamp rotation

```ini
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

## Upgrade behavior

RPM upgrades preserve a locally modified `/etc/dbtail/dbtail.conf`. A new
package configuration may be installed as `/etc/dbtail/dbtail.conf.rpmnew`;
review and merge it manually. State files are retained across upgrades and
uninstallation so collection can resume from the saved offsets.

After changing the configuration or installing a new binary:

```bash
systemctl restart dbtail
```
