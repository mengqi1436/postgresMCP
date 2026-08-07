# pg-mcp 工具清单（81 个 / 12 类）

> 所有工具均可通过 `pg_execute`（`tool_name` + `params`）统一调用，也可被 MCP 客户端直接调用。
> restricted 模式下仅 **query / metadata / monitoring** 类别及 `explain_plan` 可用。

## control（2）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `pg_list_tools` | 列出所有工具（可按类别筛选） | category |
| `pg_execute` | 统一执行入口 | tool_name, params |

## query（5）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `query` | 执行 SELECT（支持 `$n` 绑定） | sql, params |
| `query_one` | 查询取第一条（子查询包裹 LIMIT 1） | sql, params |
| `query_paginated` | 分页查询（自动追加 LIMIT/OFFSET） | sql, page, page_size |
| `count` | 统计行数 | table, where |
| `batch_query` | 批量 SELECT，逐条返回 | queries |

## dml（7）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `insert` | 单行插入（值参数化） | table, data |
| `insert_batch` | 多行单语句插入 | table, rows |
| `update` | 更新（**WHERE 必填**，SET 参数化） | table, data, where |
| `delete` | 删除（**WHERE 必填**） | table, where |
| `merge` | upsert：`INSERT ... ON CONFLICT DO UPDATE` | table, data, match_columns |
| `batch_update` | 多条不同 WHERE 更新（事务） | table, updates |
| `batch_delete` | 多条 WHERE 删除（事务） | table, wheres |

## ddl（14）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `create_table` | 建表 | table_name, columns |
| `alter_table` | 改表（ADD/MODIFY/DROP COLUMN） | table_name, operation, column, type |
| `drop_table` | 删表 | table_name, if_exists |
| `create_index` | 建索引 | index_name, table_name, columns, unique |
| `drop_index` | 删索引 | index_name, if_exists |
| `execute_ddl` | 任意 DDL | sql |
| `batch_create_tables` | 批量建表（atomic=true 真原子） | tables, atomic |
| `batch_create_indexes` | 批量建索引 | indexes, atomic |
| `batch_drop_tables` | 批量删表 | table_names, if_exists, atomic |
| `batch_drop_indexes` | 批量删索引 | index_names, if_exists, atomic |
| `create_view` | 建视图 | view_name, sql, or_replace |
| `drop_view` | 删视图 | view_name, if_exists |
| `create_sequence` | 建序列 | seq_name, start_with, increment_by, max_value, cache_size |
| `drop_sequence` | 删序列 | seq_name, if_exists |

> PG 的 DDL 可事务回滚：`atomic=true` 为真正的单事务全有或全无。例外（不能进事务块）：VACUUM、CREATE INDEX CONCURRENTLY、REINDEX CONCURRENTLY。序列 nextval 消耗不回滚。

## metadata（17）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `list_databases` | 列出数据库（含大小） | — |
| `list_schemas` | 列出 schema | include_system |
| `list_tables` | 列出表（含大小/注释） | schema |
| `list_views` | 列出视图及定义 | schema |
| `describe_table` | 表结构（列/类型/可空/默认/注释） | table_name, schema |
| `batch_describe_tables` | 批量表结构 | table_names, schema |
| `list_indexes` | 索引清单及定义 | table_name, schema |
| `search_indexes` | 按名称检索索引 | index_name, index_match(exact/prefix/like), table_name, schema |
| `describe_index` | 索引详情（类型/唯一/列/谓词） | index_name, schema |
| `list_sequences` | 序列清单 | schema |
| `list_functions` | 函数清单 | schema |
| `list_procedures` | 存储过程清单（PG11+） | schema |
| `list_triggers` | 触发器清单 | table_name |
| `list_constraints` | 表约束（PK/UK/FK/CHECK） | table_name, schema |
| `list_table_partitions` | 声明式分区信息 | table_name, schema |
| `get_table_ddl` | 反推建表 DDL + 索引定义 | table_name, schema |
| `list_extensions` | 已安装扩展 | — |

> 全部参数化查询（`$n` / `$1::regclass`），元数据以 pg_catalog 为主（全量、无权限过滤）。

## advanced（6）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `execute_sql` | 任意 SQL，按前缀自动分流 query/ddl/dml | sql, params |
| `execute_transaction` | 多语句事务（全成功提交） | statements |
| `call_function` | 调用函数（SELECT func(...)） | function_name, params |
| `call_procedure` | 调用过程（CALL，PG11+） | procedure_name, params |
| `explain_plan` | 执行计划（FORMAT JSON）；analyze 写语句自动 BEGIN/ROLLBACK 不落盘 | sql, analyze, buffers |
| `batch_execute_sql` | 批量任意 SQL | statements, atomic |

## admin（12）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `database_info` | 服务器信息（版本/地址/启动时间/大小） | — |
| `list_users` | 角色/用户清单（含成员关系） | — |
| `create_user` | 建登录用户（CREATE ROLE ... LOGIN PASSWORD） | username, password, connection_limit |
| `drop_user` | 删用户 | username |
| `grant_privilege` | 授权（GRANT ... TO） | privilege, grantee |
| `revoke_privilege` | 撤权（REVOKE ... FROM） | privilege, grantee |
| `create_role` | 建角色（不带 LOGIN） | role_name |
| `drop_role` | 删角色 | role_name |
| `list_roles` | 角色清单（同 list_users） | — |
| `list_tablespaces` | 表空间清单 | — |
| `create_tablespace` | 建表空间 | tablespace_name, datafile(目录) |
| `table_statistics` | 表访问/行数统计（pg_stat_user_tables） | table_name |

## monitoring（6）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `active_sessions` | 活跃会话（pg_stat_activity） | limit, state |
| `lock_info` | 锁等待与阻塞链（pg_locks ⋈ pg_stat_activity + pg_blocking_pids） | limit |
| `slow_queries` | 慢查询 Top-N（pg_stat_statements，缺扩展时返回安装指引） | limit |
| `tablespace_usage` | 表空间/数据库空间占用 | — |
| `instance_parameters` | 实例参数（pg_settings） | name(模糊) |
| `session_memory` | 当前会话内存上下文（PG14+） | limit |

## backup（4）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `logical_export` | pg_dump -Fc 逻辑导出 | output_file, tables, extra_args, timeout_seconds |
| `logical_import` | pg_restore 逻辑导入 | input_file, database, clean, create_db, jobs, section, extra_args |
| `physical_backup` | pg_basebackup -X stream + pg_verifybackup 自动校验 | backup_dir, backup_name, verify, timeout_seconds |
| `physical_restore` | 物理恢复预检（**confirm=true**）：校验 + 官方恢复步骤指引 | backup_dir, confirm |

## import（2）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `export_table_data` | 导出为 JSON 或 INSERT 语句 | table_name, format, where, limit |
| `batch_import_csv` | CSV 并行导入（PG 原生 COPY 协议流式推送，服务器端类型转换） | files[{csv_file, table, schema, columns, delimiter, header}], max_parallel, timeout_seconds |

## instance（6）

| 工具 | 功能 | 关键参数 |
|---|---|---|
| `create_database` | CREATE DATABASE | database_name, owner, encoding |
| `delete_database` | DROP DATABASE WITH (FORCE)（**confirm=true**） | database_name, confirm |
| `database_service_status` | pg_ctl status（需 PG_DATA_DIR） | — |
| `start_database_service` | pg_ctl start | timeout_seconds |
| `stop_database_service` | pg_ctl stop | mode(smart/fast/immediate), timeout_seconds |
| `restart_database_service` | pg_ctl restart | mode, timeout_seconds |

## 统一约定

- **返回**：JSON 文本；批量操作返回 `{success, total, ok_count, fail_count, results}`
- **错误**：业务错误带上下文信息返回（会话不中断）；restricted 模式拒绝写工具时明确提示切换 `PG_ACCESS_MODE`
- **超时**：普通工具 60s；导入/备份工具可传 `timeout_seconds`
