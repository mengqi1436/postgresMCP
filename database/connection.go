package database

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"pg-mcp/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool    Pool
	once    sync.Once
	initErr error
)

// Pool 是进程级连接池的最小接口。真实池 pgxpool.Pool 与测试用
// pgxmock.PgxPoolIface 均满足，便于用 mock 补齐 DB 层测试。
type Pool interface {
	Ping(ctx context.Context) error
	Close()
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// GetPool 返回进程级单例连接池（惰性初始化）。
// 启动时用 pool.Ping 做就绪检查（pgx 官方推荐的 readiness 检查方式）。
func GetPool(ctx context.Context) (Pool, error) {
	once.Do(func() {
		cfg := config.LoadConfig()
		if !cfg.IsValid() {
			initErr = fmt.Errorf("数据库配置无效：请设置 PG_PASSWORD（或 PG_DSN）等环境变量")
			return
		}

		poolCfg, err := pgxpool.ParseConfig(cfg.GetDSN())
		if err != nil {
			initErr = fmt.Errorf("解析连接配置失败: %w", err)
			return
		}
		poolCfg.MaxConns = 10
		poolCfg.MinConns = 1
		poolCfg.MaxConnLifetime = time.Hour
		poolCfg.MaxConnIdleTime = 30 * time.Minute
		poolCfg.HealthCheckPeriod = time.Minute

		// AfterConnect：每个新连接建立后、入池前执行。
		// 注册 numeric→string 解码，避免 float64 精度丢失（pgx wiki 明确警告）。
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			conn.TypeMap().RegisterType(&pgtype.Type{
				Name:  "numeric",
				OID:   pgtype.NumericOID,
				Codec: &numericStringCodec{},
			})
			return nil
		}

		p, err := pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			initErr = fmt.Errorf("创建连接池失败: %w", err)
			return
		}
		if err := p.Ping(ctx); err != nil {
			p.Close()
			initErr = fmt.Errorf("连接 PostgreSQL 失败 (%s): %w", cfg.MaskedDSN(), err)
			return
		}
		pool = p
		fmt.Fprintf(os.Stderr, "[数据库] 连接池就绪 (%s)\n", cfg.MaskedDSN())
	})
	return pool, initErr
}

// QueryResult 携带行数据、截断标志与精确总数。
type QueryResult struct {
	Rows      []map[string]any
	Truncated bool // 是否因达到 maxRows 被截断
	Total     int  // 总行数（截断时通过继续扫描计数得到，精确值）
}

// Query 执行 SELECT 并返回通用行数据（列名 → 值），不限制行数
// （兼容既有调用点，内部委托 QueryLimit）。
func Query(ctx context.Context, sqlStr string, args ...any) ([]map[string]any, error) {
	qr, err := QueryLimit(ctx, 0, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return qr.Rows, nil
}

// QueryLimit 执行 SELECT 并按 maxRows 限制返回行数；maxRows<=0 表示不限制。
// 达到上限后继续读取剩余行仅计数，得到精确 Total——pgx 的 rows.Close()
// 本就会 drain 剩余行以复用连接，因此计数不增加额外 DB/网络开销。
func QueryLimit(ctx context.Context, maxRows int, sqlStr string, args ...any) (*QueryResult, error) {
	p, err := GetPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	rows, err := p.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	return scanRowsLimited(rows, maxRows)
}

// Execute 执行 DML/DDL 并返回影响行数。
func Execute(ctx context.Context, sqlStr string, args ...any) (int64, error) {
	p, err := GetPool(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	tag, err := p.Exec(ctx, sqlStr, args...)
	if err != nil {
		return 0, fmt.Errorf("执行语句失败: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExecuteDDL 执行单条 DDL（PG 的 DDL 可事务回滚，此处单语句独立执行）。
func ExecuteDDL(ctx context.Context, sqlStr string) error {
	_, err := Execute(ctx, sqlStr)
	if err != nil {
		return fmt.Errorf("执行DDL失败: %w", err)
	}
	return nil
}

// Statement 事务中的一条参数化语句。
type Statement struct {
	SQL  string
	Args []any
}

// ExecuteTransaction 在同一事务中顺序执行多条语句，任一失败整体回滚。
// PG 的 DDL 也可在事务中回滚（VACUUM / CREATE INDEX CONCURRENTLY 等例外）。
func ExecuteTransaction(ctx context.Context, statements []string) error {
	stmts := make([]Statement, len(statements))
	for i, s := range statements {
		stmts[i] = Statement{SQL: s}
	}
	return ExecuteStatements(ctx, stmts)
}

// ExecuteStatements 在同一事务中顺序执行多条参数化语句，任一失败整体回滚。
func ExecuteStatements(ctx context.Context, statements []Statement) error {
	p, err := GetPool(ctx)
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 用池级 Begin（内部获取连接，事务结束自动归还），真池与 pgxmock 均支持。
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx) // 幂等：已提交/已回滚时安全

	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt.SQL, stmt.Args...); err != nil {
			return fmt.Errorf("执行语句失败 [%s]: %w", stmt.SQL, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// QueryInAbortedTx 在事务内执行查询后强制回滚。
// 官方范式：BEGIN; EXPLAIN ANALYZE <写语句>; ROLLBACK; —— 真实执行但不落盘。
func QueryInAbortedTx(ctx context.Context, sqlStr string, args ...any) ([]map[string]any, error) {
	p, err := GetPool(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	// 用池级 Begin（事务回滚后连接自动归还），真池与 pgxmock 均支持。
	tx, err := p.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx) // 无论查询成败都回滚

	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// CopyFromReader 执行 `COPY ... FROM STDIN` 并把 reader 字节流推给服务器。
// 服务器端原生解析 CSV/text（完整类型转换，等价 \copy），返回 COPY 行数。
func CopyFromReader(ctx context.Context, r io.Reader, copySQL string) (int64, error) {
	p, err := GetPool(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	conn, err := p.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取连接失败: %w", err)
	}
	defer conn.Release()

	tag, err := conn.Conn().PgConn().CopyFrom(ctx, r, copySQL)
	if err != nil {
		return 0, fmt.Errorf("COPY 导入失败: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanRows 用 rows.Values() 通用扫描任意列，构建 map 行（不限制行数）。
func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	qr, err := scanRowsLimited(rows, 0)
	if err != nil {
		return nil, err
	}
	return qr.Rows, nil
}

// scanRowsLimited 用 rows.Values() 通用扫描任意列，构建 map 行；
// maxRows<=0 不限制；达到上限后继续读取剩余行仅计数（Truncated=true）。
func scanRowsLimited(rows pgx.Rows, maxRows int) (*QueryResult, error) {
	fields := rows.FieldDescriptions()

	limited := maxRows > 0
	res := &QueryResult{}
	for rows.Next() {
		if limited && len(res.Rows) >= maxRows {
			res.Truncated = true
			res.Total++
			continue
		}
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(fields))
		for i, fd := range fields {
			row[fd.Name] = normalizeValue(fd.DataTypeOID, values[i])
		}
		res.Rows = append(res.Rows, row)
		res.Total++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// normalizeValue 把 pgx 返回值转成 JSON 友好的类型。
func normalizeValue(oid uint32, val any) any {
	switch v := val.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case [16]byte: // uuid
		return fmt.Sprintf("%x-%x-%x-%x-%x", v[0:4], v[4:6], v[6:8], v[8:10], v[10:16])
	case time.Duration:
		return v.String()
	default:
		return v
	}
}

// Close 关闭连接池。
func Close() {
	if pool != nil {
		pool.Close()
	}
}

// numericStringCodec 覆盖 pgx 默认的 numeric→float64 解码，
// 输出十进制字符串以避免精度丢失。编码路径沿用内置 NumericCodec。
type numericStringCodec struct {
	pgtype.NumericCodec
}

func (c *numericStringCodec) DecodeValue(m *pgtype.Map, oid uint32, format int16, b []byte) (any, error) {
	if format == pgtype.TextFormatCode {
		return string(b), nil
	}
	return numericBinaryToString(b)
}

// numericBinaryToString 解析 numeric 二进制格式（base-10000 数字组）为十进制字符串。
func numericBinaryToString(b []byte) (string, error) {
	if len(b) < 8 {
		return "", fmt.Errorf("numeric 二进制数据过短")
	}
	ndigits := int(int16(binary.BigEndian.Uint16(b[0:2])))
	weight := int(int16(binary.BigEndian.Uint16(b[2:4])))
	signBits := binary.BigEndian.Uint16(b[4:6])
	dscale := int(binary.BigEndian.Uint16(b[6:8]))
	if len(b) < 8+2*ndigits {
		return "", fmt.Errorf("numeric 二进制数据不完整")
	}

	if signBits == 0xC000 {
		return "NaN", nil
	}
	digits := make([]int, ndigits)
	for i := 0; i < ndigits; i++ {
		digits[i] = int(int16(binary.BigEndian.Uint16(b[8+2*i : 10+2*i])))
	}

	var sb strings.Builder
	if signBits == 0x4000 {
		sb.WriteByte('-')
	}

	// 整数部分：digits[0] 的位权为 10000^weight
	if weight >= 0 {
		for i := 0; i <= weight; i++ {
			if i < ndigits {
				if i == 0 {
					sb.WriteString(strconv.Itoa(digits[i]))
				} else {
					sb.WriteString(fmt.Sprintf("%04d", digits[i]))
				}
			} else {
				sb.WriteString("0000")
			}
		}
	} else {
		sb.WriteByte('0')
	}

	// 小数部分：按 dscale 截断/补零
	if dscale > 0 {
		var frac strings.Builder
		for i := weight + 1; i < ndigits; i++ {
			frac.WriteString(fmt.Sprintf("%04d", digits[i]))
		}
		f := frac.String()
		if len(f) > dscale {
			f = f[:dscale]
		} else {
			f += strings.Repeat("0", dscale-len(f))
		}
		sb.WriteByte('.')
		sb.WriteString(f)
	}

	return sb.String(), nil
}
