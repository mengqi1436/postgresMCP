package tools

import (
	"fmt"
	"regexp"
	"strings"

	"pg-mcp/database"
)

func init() { registerAdminTools() }

// privilegePattern 校验 GRANT/REVOKE 的权限描述（防注入：禁止分号/注释符）。
var privilegePattern = regexp.MustCompile(`^[A-Za-z0-9_ ,().'"=]+$`)

func registerAdminTools() {
	RegisterTool(ToolInfo{
		Name:        "database_info",
		Category:    "admin",
		Description: "获取 PostgreSQL 服务器基本信息（版本/当前库/地址/启动时间等）。无参数",
		Params:      []string{},
	}, handleDatabaseInfo)

	RegisterTool(ToolInfo{
		Name:        "list_users",
		Category:    "admin",
		Description: "列出所有角色/用户。无参数",
		Params:      []string{},
	}, handleListUsers)

	RegisterTool(ToolInfo{
		Name:        "create_user",
		Category:    "admin",
		Description: "创建可登录用户（CREATE ROLE ... LOGIN PASSWORD）。参数: username(必需), password(必需), connection_limit(可选)",
		Params:      []string{"username", "password", "connection_limit"},
	}, handleCreateUser)

	RegisterTool(ToolInfo{
		Name:        "drop_user",
		Category:    "admin",
		Description: "删除用户/角色。参数: username(必需)。注意: 拥有对象的角色需先 DROP OWNED BY",
		Params:      []string{"username"},
	}, handleDropUser)

	RegisterTool(ToolInfo{
		Name:        "grant_privilege",
		Category:    "admin",
		Description: "授予权限。参数: privilege(必需, 如 DBA 不适用于 PG; 例: SELECT ON TABLE t / ALL PRIVILEGES ON DATABASE d / pg_read_all_stats), grantee(必需, 用户或角色名)",
		Params:      []string{"privilege", "grantee"},
	}, handleGrantPrivilege)

	RegisterTool(ToolInfo{
		Name:        "revoke_privilege",
		Category:    "admin",
		Description: "撤销权限。参数: privilege(必需), grantee(必需)",
		Params:      []string{"privilege", "grantee"},
	}, handleRevokePrivilege)

	RegisterTool(ToolInfo{
		Name:        "create_role",
		Category:    "admin",
		Description: "创建角色（不自动带 LOGIN）。参数: role_name(必需)",
		Params:      []string{"role_name"},
	}, handleCreateRole)

	RegisterTool(ToolInfo{
		Name:        "drop_role",
		Category:    "admin",
		Description: "删除角色。参数: role_name(必需)",
		Params:      []string{"role_name"},
	}, handleDropRole)

	RegisterTool(ToolInfo{
		Name:        "list_roles",
		Category:    "admin",
		Description: "列出所有角色及属性。无参数",
		Params:      []string{},
	}, handleListRoles)

	RegisterTool(ToolInfo{
		Name:        "list_tablespaces",
		Category:    "admin",
		Description: "列出所有表空间及使用大小。无参数",
		Params:      []string{},
	}, handleListTablespaces)

	RegisterTool(ToolInfo{
		Name:        "create_tablespace",
		Category:    "admin",
		Description: "创建表空间。参数: tablespace_name(必需), datafile(必需, 服务器端目录路径, 须为 postgres 属主且已存在)",
		Params:      []string{"tablespace_name", "datafile"},
	}, handleCreateTablespace)

	RegisterTool(ToolInfo{
		Name:        "table_statistics",
		Category:    "admin",
		Description: "表访问/行数/大小统计（pg_stat_user_tables）。参数: table_name(可选, 不传返回全部用户表)",
		Params:      []string{"table_name"},
	}, handleTableStatistics)
}

func handleDatabaseInfo(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, `
SELECT version() AS version,
       current_database() AS current_database,
       current_user AS current_user,
       inet_server_addr()::text AS server_addr,
       inet_server_port() AS server_port,
       pg_catalog.current_setting('max_connections') AS max_connections,
       pg_catalog.pg_postmaster_start_time() AS start_time,
       pg_catalog.pg_size_pretty(pg_catalog.pg_database_size(current_database())) AS database_size,
       pg_catalog.pg_is_in_recovery() AS in_recovery`)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("无法获取服务器信息")
	}
	return rows[0], nil
}

const rolesSelect = `
SELECT r.rolname AS role_name, r.rolsuper AS is_superuser, r.rolinherit AS inherit,
       r.rolcreaterole AS can_create_role, r.rolcreatedb AS can_create_db,
       r.rolcanlogin AS can_login, r.rolreplication AS replication,
       r.rolconnlimit AS connection_limit, r.rolvaliduntil AS valid_until,
       ARRAY(SELECT b.rolname FROM pg_catalog.pg_auth_members m
             JOIN pg_catalog.pg_roles b ON b.oid = m.roleid
             WHERE m.member = r.oid) AS member_of
FROM pg_catalog.pg_roles r`

func handleListUsers(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, rolesSelect+" ORDER BY r.rolname")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"users": rows, "count": len(rows)}, nil
}

func handleListRoles(params map[string]interface{}) (interface{}, error) {
	return handleListUsers(params)
}

func handleCreateUser(params map[string]interface{}) (interface{}, error) {
	username := getString(params, "username")
	if err := validateIdentifier(username); err != nil {
		return nil, fmt.Errorf("用户名校验失败: %w", err)
	}
	password := getString(params, "password")
	if password == "" {
		return nil, fmt.Errorf("参数 password 是必需的")
	}

	// CREATE ROLE 的 PASSWORD 不支持 $n 参数绑定，用字面量引用 + 标识符校验兜底
	sqlStr := fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s",
		quoteIdent(username), quoteLiteral(password))
	if limit := getIntDefault(params, "connection_limit", 0); limit > 0 {
		sqlStr += fmt.Sprintf(" CONNECTION LIMIT %d", limit)
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "username": username}, nil
}

func handleDropUser(params map[string]interface{}) (interface{}, error) {
	username := getString(params, "username")
	if err := validateIdentifier(username); err != nil {
		return nil, fmt.Errorf("用户名校验失败: %w", err)
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, "DROP ROLE "+quoteIdent(username)); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "username": username}, nil
}

func handleGrantPrivilege(params map[string]interface{}) (interface{}, error) {
	privilege := strings.TrimSpace(getString(params, "privilege"))
	grantee := getString(params, "grantee")
	if privilege == "" {
		return nil, fmt.Errorf("参数 privilege 是必需的")
	}
	if !privilegePattern.MatchString(privilege) {
		return nil, fmt.Errorf("privilege 含非法字符")
	}
	if err := validateIdentifier(grantee); err != nil {
		return nil, fmt.Errorf("grantee 校验失败: %w", err)
	}

	sqlStr := fmt.Sprintf("GRANT %s TO %s", privilege, quoteIdent(grantee))

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleRevokePrivilege(params map[string]interface{}) (interface{}, error) {
	privilege := strings.TrimSpace(getString(params, "privilege"))
	grantee := getString(params, "grantee")
	if privilege == "" {
		return nil, fmt.Errorf("参数 privilege 是必需的")
	}
	if !privilegePattern.MatchString(privilege) {
		return nil, fmt.Errorf("privilege 含非法字符")
	}
	if err := validateIdentifier(grantee); err != nil {
		return nil, fmt.Errorf("grantee 校验失败: %w", err)
	}

	sqlStr := fmt.Sprintf("REVOKE %s FROM %s", privilege, quoteIdent(grantee))

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleCreateRole(params map[string]interface{}) (interface{}, error) {
	roleName := getString(params, "role_name")
	if err := validateIdentifier(roleName); err != nil {
		return nil, fmt.Errorf("角色名校验失败: %w", err)
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, "CREATE ROLE "+quoteIdent(roleName)); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "role_name": roleName}, nil
}

func handleDropRole(params map[string]interface{}) (interface{}, error) {
	roleName := getString(params, "role_name")
	if err := validateIdentifier(roleName); err != nil {
		return nil, fmt.Errorf("角色名校验失败: %w", err)
	}

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, "DROP ROLE "+quoteIdent(roleName)); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "role_name": roleName}, nil
}

func handleListTablespaces(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	rows, err := database.Query(ctx, `
SELECT t.spcname AS tablespace_name,
       pg_catalog.pg_get_userbyid(t.spcowner) AS owner,
       pg_catalog.pg_size_pretty(pg_catalog.pg_tablespace_size(t.oid)) AS size,
       pg_catalog.pg_tablespace_location(t.oid) AS location
FROM pg_catalog.pg_tablespace t
ORDER BY t.spcname`)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"tablespaces": rows, "count": len(rows)}, nil
}

func handleCreateTablespace(params map[string]interface{}) (interface{}, error) {
	name := getString(params, "tablespace_name")
	if err := validateIdentifier(name); err != nil {
		return nil, fmt.Errorf("表空间名校验失败: %w", err)
	}
	location := getString(params, "datafile")
	if location == "" {
		return nil, fmt.Errorf("参数 datafile 是必需的（服务器端目录路径）")
	}

	sqlStr := fmt.Sprintf("CREATE TABLESPACE %s LOCATION %s",
		quoteIdent(name), quoteLiteral(location))

	ctx, cancel := toolContext()
	defer cancel()

	if err := database.ExecuteDDL(ctx, sqlStr); err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "sql": sqlStr}, nil
}

func handleTableStatistics(params map[string]interface{}) (interface{}, error) {
	ctx, cancel := toolContext()
	defer cancel()

	sqlStr := `
SELECT s.schemaname AS schema_name, s.relname AS table_name,
       s.seq_scan, s.idx_scan, s.n_live_tup AS live_rows,
       s.n_tup_ins AS inserts, s.n_tup_upd AS updates, s.n_tup_del AS deletes,
       s.last_vacuum, s.last_autovacuum, s.last_analyze, s.last_autoanalyze,
       pg_catalog.pg_size_pretty(pg_catalog.pg_total_relation_size(s.relid)) AS total_size
FROM pg_catalog.pg_stat_user_tables s`

	var args []interface{}
	if table := getString(params, "table_name"); table != "" {
		args = append(args, table)
		sqlStr += fmt.Sprintf(" WHERE s.relname = $%d", len(args))
	}
	sqlStr += " ORDER BY s.schemaname, s.relname"

	rows, err := database.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"statistics": rows, "count": len(rows)}, nil
}
