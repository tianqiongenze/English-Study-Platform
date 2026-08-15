// Package db —— ESP Go 服务统一数据访问层。
// #140 全服务切 CRDB + 用户要求保留读写分离能力(会话22-9 决策)。
//
// 提供两级 API:
//   - 单池:MustConnectDB(dsn, svc) → *sql.DB,供简单服务(listening/reading/gamification)。
//   - 双池:MustConnectReadWrite(readDSN, writeDSN, svc) → *Pool,
//           保留 conversation-go 的 Write/Read/Enabled/forcedPrimary/fallback 语义(铁律① 下沉共享包)。
//
// 连接驱动:pgx(CRDB postgres 协议)。DSN 用 postgres://root@host:26257/db?sslmode=disable。
// CRDB 单端点阶段 read/write DSN 同址(crdb-public:26257);多区域阶段读池切 follower 节点,代码零改动。
// 方案C(会话22-9):读池 DSN 注入 default_transaction_use_follower_reads=on(startup 会话默认),
// 使读池所有 SELECT 走任意副本的 -5s 快照(已过 Raft 多数派提交 → 强一致)。写池不注入,保持强一致写。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// driverFor 按 DSN 前缀自动选择驱动:postgres:// → pgx(CRDB),否则 mysql(TiDB)。
// 双驱动保留 TiDB 读写分离回切路径(用户指令 #140):服务设 READ_DB_URL/WRITE_DB_URL 为 mysql DSN
// 即走 mysql 驱动 + 双池(写 tidb-primary / 读 tidb-follower),代码零改动(SQL 方言为明确边界)。
func driverFor(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") {
		return "pgx"
	}
	return "mysql"
}

// ================= 单池 =================

// MustConnectDB 打开连接池并 Ping,失败 log.Fatal。驱动按 DSN 前缀自动识别(pgx/mysql)。
func MustConnectDB(dsn, serviceName string) *sql.DB {
	d, err := sql.Open(driverFor(dsn), dsn)
	if err != nil {
		log.Fatalf("[%s] db open: %v", serviceName, err)
	}
	d.SetMaxOpenConns(10)
	d.SetMaxIdleConns(2)
	if err := d.Ping(); err != nil {
		log.Fatalf("[%s] db ping: %v", serviceName, err)
	}
	return d
}

// DefaultDSN 拼装 CRDB postgres DSN。host 例 "crdb-public:26257",db 例 "esp_listening"。
func DefaultDSN(host, db string) string {
	return fmt.Sprintf("postgres://root@%s/%s?sslmode=disable", host, db)
}

// followerReadParam CRDB 读池会话默认:开启 follower read。
// 读池所有事务(含隐式单语句)以 AS OF SYSTEM TIME '-5s' 读任意副本 → 数据已过 Raft 多数派提交,
// 返回强一致快照(方案C 原生读写分离)。多区域阶段 CRDB_READ_HOST 指到 follower 节点时读本地副本。
func followerReadParam(dsn string) string {
	return dsn + "&default_transaction_use_follower_reads=on"
}

// ReadWriteDSN 返回读写 DSN 对。
//   - 缺省/DB_DRIVER=pgx:CRDB 单端点同址(crdb-public:26257)。CRDB_HOST/CRDB_PORT 覆盖写址;
//     CRDB_READ_HOST 覆盖读址——多区域阶段仅设 CRDB_READ_HOST=follower 即切读池,代码零改动。
//     读池注入 followerReadParam(见上);写池不注入,保持强一致。
//   - DB_DRIVER=mysql:TiDB 读写分离(写 tidb-primary / 读 tidb-follower),WRITE_DB_HOST/READ_DB_HOST 可覆盖。
func ReadWriteDSN(db string) (read, write string) {
	if os.Getenv("DB_DRIVER") == "mysql" {
		return mysqlDSN("READ_DB_HOST", "tidb-follower", db), mysqlDSN("WRITE_DB_HOST", "tidb-primary", db)
	}
	host := os.Getenv("CRDB_HOST")
	if host == "" {
		host = "crdb-public"
	}
	port := os.Getenv("CRDB_PORT")
	if port == "" {
		port = "26257"
	}
	readHost := os.Getenv("CRDB_READ_HOST")
	if readHost == "" {
		readHost = host
	}
	readDSN := DefaultDSN(readHost+":"+port, db)
	// 读池默认开启 follower-read;读-改-写类服务(SRS 等)须强一致读时设 CRDB_FOLLOWER_READ=false 回退。
	if os.Getenv("CRDB_FOLLOWER_READ") != "false" {
		readDSN = followerReadParam(readDSN)
	}
	return readDSN, DefaultDSN(host+":"+port, db)
}

// mysqlDSN 拼装 TiDB DSN(root@tcp(host:port)/db?parseTime=true),host 由 env 可覆盖。
func mysqlDSN(hostEnv, defaultHost, db string) string {
	host := os.Getenv(hostEnv)
	if host == "" {
		host = defaultHost
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "4000"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "root"
	}
	pass := os.Getenv("DB_PASS")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, db)
}

// ================= 双池(读写分离) =================

// Pool 封装读写分离双连接池。
type Pool struct {
	write   *sql.DB
	read    *sql.DB
	enabled bool
}

// MustConnectReadWrite 建立读写分离双连接池并 Ping,失败 log.Fatal。
// 驱动按各 DSN 前缀自动识别(双驱动回切 TiDB)。
func MustConnectReadWrite(readDSN, writeDSN, serviceName string) *Pool {
	w, err := sql.Open(driverFor(writeDSN), writeDSN)
	if err != nil {
		log.Fatalf("[%s] open write db failed: %v", serviceName, err)
	}
	w.SetMaxOpenConns(10)
	w.SetMaxIdleConns(2)
	w.SetConnMaxLifetime(30 * time.Minute)

	r, err := sql.Open(driverFor(readDSN), readDSN)
	if err != nil {
		log.Fatalf("[%s] open read db failed: %v", serviceName, err)
	}
	r.SetMaxOpenConns(10)
	r.SetMaxIdleConns(2)
	r.SetConnMaxLifetime(30 * time.Minute)

	if err := w.Ping(); err != nil {
		log.Fatalf("[%s] write db ping: %v", serviceName, err)
	}
	if err := r.Ping(); err != nil {
		log.Fatalf("[%s] read db ping: %v", serviceName, err)
	}
	slog.Info("db pools ready", "write", writeDSN, "read", readDSN)
	return &Pool{write: w, read: r, enabled: true}
}

// Write 执行写操作(INSERT/UPDATE/DELETE/DDL)走写池,返回 affected rows。
func (p *Pool) Write(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if p == nil || p.write == nil {
		return nil, fmt.Errorf("write db not initialized")
	}
	return p.write.ExecContext(ctx, query, args...)
}

// Read 执行读操作走读池。forcedPrimary=true 强制走写池(写后强一致读)。
// 读池不可达时自动 fallback 到写池(容错,只 warn)。
func (p *Pool) Read(ctx context.Context, query string, forcedPrimary bool, args ...any) (*sql.Rows, error) {
	if p == nil || p.write == nil {
		return nil, fmt.Errorf("no db pool available")
	}
	pool := p.read
	if forcedPrimary || pool == nil {
		pool = p.write
	}
	rows, err := pool.QueryContext(ctx, query, args...)
	if err != nil && !forcedPrimary && p.read != nil {
		slog.Warn("read from follower failed, fallback to primary", "err", err)
		return p.write.QueryContext(ctx, query, args...)
	}
	return rows, err
}

// Enabled 返回读写分离是否启用。
func (p *Pool) Enabled() bool { return p != nil && p.enabled && p.write != nil }

// WriteDB 暴露写池(供需要 *sql.DB 直接调用的场景)。
func (p *Pool) WriteDB() *sql.DB { return p.write }

// ReadDB 暴露读池。
func (p *Pool) ReadDB() *sql.DB { return p.read }

// ================= 全局默认池(包级 API,兼容 gateway-go/conversation-go) =================

var (
	globalPool    *Pool
	globalOnce    sync.Once
	globalEnabled = os.Getenv("DB_ENABLED") != "false"
)

// InitGlobal 初始化全局读写池(进程级单例)。gateway-go 等服务 main 启动时调用一次。
// DB_ENABLED=false 时 no-op(回退纯 Redis/LLM 模式)。
func InitGlobal(readDSN, writeDSN, serviceName string) {
	if !globalEnabled {
		slog.Info("db disabled (DB_ENABLED=false), running in Redis/LLM-only mode")
		return
	}
	globalOnce.Do(func() { globalPool = MustConnectReadWrite(readDSN, writeDSN, serviceName) })
}

// Write 全局写(委托 globalPool)。DB_ENABLED=false 时 no-op。
func Write(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !globalEnabled || globalPool == nil {
		return nil, nil
	}
	return globalPool.Write(ctx, query, args...)
}

// Read 全局读(委托 globalPool)。DB_ENABLED=false 时 no-op。
func Read(ctx context.Context, query string, forcedPrimary bool, args ...any) (*sql.Rows, error) {
	if !globalEnabled || globalPool == nil {
		return nil, nil
	}
	return globalPool.Read(ctx, query, forcedPrimary, args...)
}

// Enabled 返回全局读写分离是否启用。
func Enabled() bool { return globalEnabled && globalPool != nil }
