// Package db 提供 gateway-go 读写分离数据访问(与 Java 侧 esp-rw-java / Python 侧 esp-db-py 语义对齐)。
//
// 两个连接池:write -> tidb-primary(主写),read -> tidb-follower(从读)。
// 调用方显式选择 Write / Read 方法;写后强一致读用 ReadForced(强制走主库)。
// 从库不可达时自动 fallback 到主库读(容错,只 warn)。
//
// feature flag:DB_ENABLED=false 时所有方法 no-op(回退安全,供纯 Redis/LLM 模式降级)。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	enabled       = os.Getenv("DB_ENABLED") != "false"
	writeDB       *sql.DB
	readDB        *sql.DB
	writeHost     = os.Getenv("WRITE_DB_HOST")
	readHost      = os.Getenv("READ_DB_HOST")
	writeUser     = getenvDefault("WRITE_DB_USER", "root")
	writePass     = os.Getenv("WRITE_DB_PASS")
	writeDBName   = getenvDefault("WRITE_DB_NAME", "esp")
	readUser      = getenvDefault("READ_DB_USER", "root")
	readPass      = os.Getenv("READ_DB_PASS")
	readDBName    = getenvDefault("READ_DB_NAME", "esp")
	writePort     = getenvDefault("WRITE_DB_PORT", "4000")
	readPort      = getenvDefault("READ_DB_PORT", "4000")
	maxOpen       = getenvDefaultInt("DB_POOL_SIZE", 10)
	maxIdle       = getenvDefaultInt("DB_POOL_MIN", 2)
	initOnce      sync.Once
)

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvDefaultInt(k string, def int) int {
	var v int
	if _, err := fmt.Sscanf(os.Getenv(k), "%d", &v); err == nil && v > 0 {
		return v
	}
	return def
}

func dsn(host, port, user, pass, dbname string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local&timeout=10s&readTimeout=30s", user, pass, host, port, dbname)
}

func ensurePools() {
	initOnce.Do(func() {
		if !enabled {
			slog.Info("db disabled (DB_ENABLED=false), gateway-go runs in Redis/LLM-only mode")
			return
		}
		writeHost = getenvDefault("WRITE_DB_HOST", "tidb-primary")
		readHost = getenvDefault("READ_DB_HOST", "tidb-follower")
		w, err := sql.Open("mysql", dsn(writeHost, writePort, writeUser, writePass, writeDBName))
		if err != nil {
			slog.Error("open write db failed", "err", err)
			return
		}
		w.SetMaxOpenConns(maxOpen)
		w.SetMaxIdleConns(maxIdle)
		w.SetConnMaxLifetime(30 * time.Minute)
		writeDB = w

		r, err := sql.Open("mysql", dsn(readHost, readPort, readUser, readPass, readDBName))
		if err != nil {
			slog.Error("open read db failed", "err", err)
			return
		}
		r.SetMaxOpenConns(maxOpen)
		r.SetMaxIdleConns(maxIdle)
		r.SetConnMaxLifetime(30 * time.Minute)
		readDB = r
		slog.Info("db pools ready", "write", writeHost+":"+writePort, "read", readHost+":"+readPort)
	})
}

// Write 执行写操作(INSERT/UPDATE/DELETE/DDL)走主库 tidb-primary,返回 affected rows。
func Write(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !enabled {
		return nil, nil
	}
	ensurePools()
	if writeDB == nil {
		return nil, fmt.Errorf("write db not initialized")
	}
	return writeDB.ExecContext(ctx, query, args...)
}

// Read 执行读操作走从库 tidb-follower。forcedPrimary=true 强制走主库(写后强一致读)。
// 从库不可达时自动 fallback 到主库读(容错)。
func Read(ctx context.Context, query string, forcedPrimary bool, args ...any) (*sql.Rows, error) {
	if !enabled {
		return nil, nil
	}
	ensurePools()
	pool := readDB
	if forcedPrimary || pool == nil {
		pool = writeDB
	}
	if pool == nil {
		return nil, fmt.Errorf("no db pool available")
	}
	rows, err := pool.QueryContext(ctx, query, args...)
	if err != nil && !forcedPrimary && readDB != nil {
		slog.Warn("read from follower failed, fallback to primary", "err", err)
		return writeDB.QueryContext(ctx, query, args...)
	}
	return rows, err
}

// Enabled 返回读写分离是否启用。
func Enabled() bool { return enabled && writeDB != nil }
