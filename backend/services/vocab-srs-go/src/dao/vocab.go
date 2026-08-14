// Package dao 封装 vocab-srs-go 对 TiDB esp_vocab schema 的读写。
// 复用 ESP 基础设施:ExternalName 短名 tidb-primary(写)/ tidb-follower(读),
// 通过 env READ_DB_URL / WRITE_DB_URL 注入完整 DSN(优先),否则用 TIDB_HOST/TIDB_PORT 拼装。
package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// VocabRow 对应 esp_vocab.vocabulary 表(查询用)。
type VocabRow struct {
	ID            int64           `json:"id"`
	Word          string          `json:"word"`
	Phonetic      sql.NullString  `json:"-"`
	Pos           sql.NullString  `json:"-"`
	Definition    string          `json:"definition"`
	DefinitionZh  sql.NullString  `json:"-"`
	ExamplesJSON  json.RawMessage `json:"examples,omitempty"`
	CefrLevel     sql.NullString  `json:"-"`
}

// UserVocabRow 对应 esp_vocab.user_vocabulary 表(SRS 状态)。
type UserVocabRow struct {
	ID            int64
	UserID        int64
	WordID        int64
	SrsStage      int
	IntervalDays  int
	EaseFactor    float64
	DueAt         time.Time
	ReviewCount   int
	LapseCount    int
	Status        string
	LastReviewed  sql.NullTime
	CreatedAt     time.Time
}

// VocabDAO 持有读写两个 *sql.DB(读写分离)。
type VocabDAO struct {
	read  *sql.DB
	write *sql.DB
}

// NewVocabDAO 根据 DSN 建立读写两个连接池。
// readDSN/writeDSN 为完整 mysql DSN;若读库不可用可与写库一致。
func NewVocabDAO(readDSN, writeDSN string) (*VocabDAO, error) {
	r, err := openDB(readDSN)
	if err != nil {
		return nil, fmt.Errorf("open read db: %w", err)
	}
	w, err := openDB(writeDSN)
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("open write db: %w", err)
	}
	return &VocabDAO{read: r, write: w}, nil
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

// Close 关闭读写连接池。
func (d *VocabDAO) Close() error {
	_ = d.read.Close()
	return d.write.Close()
}

// Ping 检查读库连通性(用于 readiness)。
func (d *VocabDAO) Ping(ctx context.Context) error {
	return d.read.PingContext(ctx)
}

// GetVocabByID 按主键查词条。
func (d *VocabDAO) GetVocabByID(ctx context.Context, id int64) (*VocabRow, error) {
	row := d.read.QueryRowContext(ctx,
		`SELECT id, word, phonetic, pos, definition, definition_zh, examples_json, cefr_level
		 FROM esp_vocab.vocabulary WHERE id = ?`, id)
	v := &VocabRow{}
	if err := row.Scan(&v.ID, &v.Word, &v.Phonetic, &v.Pos, &v.Definition,
		&v.DefinitionZh, &v.ExamplesJSON, &v.CefrLevel); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

// CreateVocab 插入新词条(若 word 唯一键冲突返回错误)。
func (d *VocabDAO) CreateVocab(ctx context.Context, word, phonetic, pos, definition, definitionZh, cefrLevel string, examples json.RawMessage) (int64, error) {
	res, err := d.write.ExecContext(ctx,
		`INSERT INTO esp_vocab.vocabulary (word, phonetic, pos, definition, definition_zh, examples_json, cefr_level)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		word, nullStr(phonetic), nullStr(pos), definition, nullStr(definitionZh), examples, nullStr(cefrLevel))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserVocab 取某用户某词的 SRS 状态;不存在返回 nil。
func (d *VocabDAO) GetUserVocab(ctx context.Context, userID, wordID int64) (*UserVocabRow, error) {
	row := d.read.QueryRowContext(ctx,
		`SELECT id, user_id, word_id, srs_stage, interval_days, ease_factor, due_at,
		        review_count, lapse_count, status, last_reviewed, created_at
		 FROM esp_vocab.user_vocabulary WHERE user_id = ? AND word_id = ?`, userID, wordID)
	u := &UserVocabRow{}
	if err := row.Scan(&u.ID, &u.UserID, &u.WordID, &u.SrsStage, &u.IntervalDays,
		&u.EaseFactor, &u.DueAt, &u.ReviewCount, &u.LapseCount, &u.Status,
		&u.LastReviewed, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// UpsertUserVocab 新建或更新某用户某词的 SRS 状态(TiDB 无 ON DUPLICATE,走显式 upsert)。
func (d *VocabDAO) UpsertUserVocab(ctx context.Context, u *UserVocabRow) error {
	_, err := d.write.ExecContext(ctx,
		`INSERT INTO esp_vocab.user_vocabulary
		   (user_id, word_id, srs_stage, interval_days, ease_factor, due_at,
		    review_count, lapse_count, status, last_reviewed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   srs_stage=VALUES(srs_stage),
		   interval_days=VALUES(interval_days),
		   ease_factor=VALUES(ease_factor),
		   due_at=VALUES(due_at),
		   review_count=VALUES(review_count),
		   lapse_count=VALUES(lapse_count),
		   status=VALUES(status),
		   last_reviewed=VALUES(last_reviewed)`,
		u.UserID, u.WordID, u.SrsStage, u.IntervalDays, u.EaseFactor, u.DueAt,
		u.ReviewCount, u.LapseCount, u.Status, u.LastReviewed)
	return err
}

// ListDueWords 查询某用户当前到期的词列表(连表 vocabulary 取词面信息),limit 控制数量。
func (d *VocabDAO) ListDueWords(ctx context.Context, userID int64, now time.Time, limit int) ([]VocabRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.read.QueryContext(ctx,
		`SELECT v.id, v.word, v.phonetic, v.pos, v.definition, v.definition_zh, v.examples_json, v.cefr_level
		 FROM esp_vocab.user_vocabulary uv
		 JOIN esp_vocab.vocabulary v ON v.id = uv.word_id
		 WHERE uv.user_id = ? AND uv.due_at <= ?
		 ORDER BY uv.due_at ASC
		 LIMIT ?`, userID, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VocabRow
	for rows.Next() {
		var v VocabRow
		if err := rows.Scan(&v.ID, &v.Word, &v.Phonetic, &v.Pos, &v.Definition,
			&v.DefinitionZh, &v.ExamplesJSON, &v.CefrLevel); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteVocab 删除词条(管理操作,谨慎使用)。
func (d *VocabDAO) DeleteVocab(ctx context.Context, id int64) error {
	_, err := d.write.ExecContext(ctx, `DELETE FROM esp_vocab.vocabulary WHERE id = ?`, id)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
