// Package db —— ESP Go 服务统一 TiDB 连接池(会话22-3 债1+2)。
// 消除 listening/reading/gamification 三处逐字复制的 mustConnectDB + 读写分离 DSN 拼装。
package db

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/go-sql-driver/mysql"
)

// MustConnectDB 打开 mysql 连接池并 Ping,失败 log.Fatal。连接池默认 SetMaxOpenConns=10 / SetMaxIdleConns=2,
// 对齐 listening-go/reading-go/gamification-go 原 mustConnectDB 行为(无行为变更)。
func MustConnectDB(dsn, serviceName string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("[%s] db open: %v", serviceName, err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		log.Fatalf("[%s] db ping: %v", serviceName, err)
	}
	return db
}

// DefaultDSN 拼装 TiDB DSN,root 无密码,parseTime=true。host 例 "tidb-primary:4000",db 例 "esp_listening"。
func DefaultDSN(host, db string) string {
	return fmt.Sprintf("root:@tcp(%s)/%s?parseTime=true", host, db)
}

// ReadWriteDSN 一次返回读写分离 DSN 对(从库 tidb-follower / 主库 tidb-primary),对齐各服务原默认值。
func ReadWriteDSN(db string) (read, write string) {
	return DefaultDSN("tidb-follower:4000", db), DefaultDSN("tidb-primary:4000", db)
}

// 为避免未使用 import 报错占位(mysql driver 由调用方 _ import,本包仅用其 cfg 类型时引用)。
var _ = mysql.Config{}
