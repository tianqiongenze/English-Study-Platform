package db

import (
	"strings"
	"testing"
)

// TestReadWriteDSN_FollowerReadOnCRDB 验证方案C:CRDB 读池 DSN 注入
// default_transaction_use_follower_reads=on(startup 参数),写池不注入(强一致写)。
func TestReadWriteDSN_FollowerReadOnCRDB(t *testing.T) {
	t.Setenv("DB_DRIVER", "")
	t.Setenv("CRDB_HOST", "")
	t.Setenv("CRDB_PORT", "")
	t.Setenv("CRDB_READ_HOST", "")
	read, write := ReadWriteDSN("esp_speaking")
	if !strings.HasPrefix(read, "postgres://") {
		t.Fatalf("read DSN should be postgres(CRDB), got: %s", read)
	}
	if !strings.Contains(read, "default_transaction_use_follower_reads=on") {
		t.Fatalf("read DSN must enable follower_read, got: %s", read)
	}
	if strings.Contains(write, "default_transaction_use_follower_reads") {
		t.Fatalf("write DSN must stay strong-consistent, got: %s", write)
	}
	if !strings.Contains(read, "crdb-public:26257") {
		t.Fatalf("default read host should be crdb-public:26257, got: %s", read)
	}
}

// TestReadWriteDSN_ReadHostOverride 验证 CRDB_READ_HOST 覆盖读址(多区域阶段切 follower 节点),
// follower-read 参数仍注入。
func TestReadWriteDSN_ReadHostOverride(t *testing.T) {
	t.Setenv("DB_DRIVER", "")
	t.Setenv("CRDB_HOST", "crdb-primary:26257")
	t.Setenv("CRDB_READ_HOST", "crdb-follower-zone2:26257")
	read, write := ReadWriteDSN("esp_speaking")
	if !strings.Contains(read, "crdb-follower-zone2:26257") {
		t.Fatalf("read DSN should use CRDB_READ_HOST, got: %s", read)
	}
	if !strings.Contains(read, "default_transaction_use_follower_reads=on") {
		t.Fatalf("override read DSN must keep follower_read, got: %s", read)
	}
	if !strings.Contains(write, "crdb-primary:26257") {
		t.Fatalf("write DSN should use CRDB_HOST, got: %s", write)
	}
}

// TestReadWriteDSN_FollowerReadOptOut 验证 CRDB_FOLLOWER_READ=false 回退强一致读
// (读-改-写类服务如 vocab SRS 需要;默认开启由 FollowerReadOnCRDB 覆盖)。
func TestReadWriteDSN_FollowerReadOptOut(t *testing.T) {
	t.Setenv("DB_DRIVER", "")
	t.Setenv("CRDB_HOST", "")
	t.Setenv("CRDB_PORT", "")
	t.Setenv("CRDB_READ_HOST", "")
	t.Setenv("CRDB_FOLLOWER_READ", "false")
	read, _ := ReadWriteDSN("esp_speaking")
	if strings.Contains(read, "default_transaction_use_follower_reads") {
		t.Fatalf("read DSN must NOT carry follower_read when CRDB_FOLLOWER_READ=false, got: %s", read)
	}
}

// TestReadWriteDSN_MySQLPathUnchanged 验证 TiDB 回切路径(mysql 驱动)不注入 follower-read 参数,
// 保持既有读写分离(写 tidb-primary / 读 tidb-follower)。
func TestReadWriteDSN_MySQLPathUnchanged(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("READ_DB_HOST", "")
	t.Setenv("WRITE_DB_HOST", "")
	t.Setenv("DB_PORT", "")
	read, write := ReadWriteDSN("esp_speaking")
	if strings.Contains(read, "default_transaction_use_follower_reads") {
		t.Fatalf("mysql read DSN must not carry CRDB follower_read param, got: %s", read)
	}
	if !strings.HasPrefix(read, "root:@tcp(") {
		t.Fatalf("mysql read DSN expected, got: %s", read)
	}
	if !strings.Contains(read, "tidb-follower") {
		t.Fatalf("mysql read DSN should point to tidb-follower, got: %s", read)
	}
	if !strings.Contains(write, "tidb-primary") {
		t.Fatalf("mysql write DSN should point to tidb-primary, got: %s", write)
	}
}
