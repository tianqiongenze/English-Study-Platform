// Package dao 封装 vocab-srs-go 对 CRDB esp_vocab schema 的读写(#140 全服务切 CRDB)。
// 复用 esp-common-go/db 双池(读写分离),DSN 经 env READ_DB_URL / WRITE_DB_URL 注入(默认 crdb-public:26257)。
package dao

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"esp/common-go/db"
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
	// P0-D2(2026-08-21):乐观锁版本列。并发 review 同一(user,word)时,UpsertUserVocab 的
	// ON CONFLICT DO UPDATE 加 WHERE version=$old,旧版本冲突返 0 行让上层重读重算,防 lost update。
	Version int
}

// VocabDAO 持有 esp-common-go/db 读写双池(读写分离)。
type VocabDAO struct {
	pool *db.Pool
}

// NewVocabDAO 建立读写双池。readDSN/writeDSN 为完整 postgres DSN;
// 失败时 esp-common-go/db 内部 log.Fatal(启动即暴露 DB 故障)。
func NewVocabDAO(readDSN, writeDSN string) (*VocabDAO, error) {
	pool := db.MustConnectReadWrite(readDSN, writeDSN, "vocab-srs-go")
	return &VocabDAO{pool: pool}, nil
}

// Close 关闭读写连接池。
func (d *VocabDAO) Close() error {
	if d.pool == nil {
		return nil
	}
	if w := d.pool.WriteDB(); w != nil {
		_ = w.Close()
	}
	if r := d.pool.ReadDB(); r != nil {
		_ = r.Close()
	}
	return nil
}

// Ping 检查读库连通性(用于 readiness)。
func (d *VocabDAO) Ping(ctx context.Context) error {
	return d.pool.ReadDB().PingContext(ctx)
}

// GetVocabByID 按主键查词条。
func (d *VocabDAO) GetVocabByID(ctx context.Context, id int64) (*VocabRow, error) {
	row := d.pool.ReadDB().QueryRowContext(ctx,
		`SELECT id, word, phonetic, pos, definition, definition_zh, examples_json, cefr_level
		 FROM vocabulary WHERE id = $1`, id)
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
	// jsonb 列:JSON 以字符串传参 + ::jsonb 显式转换(pgx 对 []byte 默认 bytea,故转 string)。
	var ex any
	if len(examples) > 0 {
		ex = string(examples)
	}
	var id int64
	err := d.pool.WriteDB().QueryRowContext(ctx,
		`INSERT INTO vocabulary (word, phonetic, pos, definition, definition_zh, examples_json, cefr_level)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7) RETURNING id`,
		word, nullStr(phonetic), nullStr(pos), definition, nullStr(definitionZh), ex, nullStr(cefrLevel)).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetUserVocab 取某用户某词的 SRS 状态;不存在返回 nil。
// P0-D2(2026-08-21):读 follower(ReadDB)有复制延迟,Review 流程的 read-modify-write
// 读旧值算 SM-2 基线错误。Review 调用方在并发场景需用 WriteDB 强一致读或乐观重试兜底。
func (d *VocabDAO) GetUserVocab(ctx context.Context, userID, wordID int64) (*UserVocabRow, error) {
	row := d.pool.ReadDB().QueryRowContext(ctx,
		`SELECT id, user_id, word_id, srs_stage, interval_days, ease_factor, due_at,
		        review_count, lapse_count, status, last_reviewed, created_at, version
		 FROM user_vocabulary WHERE user_id = $1 AND word_id = $2`, userID, wordID)
	u := &UserVocabRow{}
	if err := row.Scan(&u.ID, &u.UserID, &u.WordID, &u.SrsStage, &u.IntervalDays,
		&u.EaseFactor, &u.DueAt, &u.ReviewCount, &u.LapseCount, &u.Status,
		&u.LastReviewed, &u.CreatedAt, &u.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// UpsertUserVocab 新建或更新某用户某词的 SRS 状态(CRDB ON CONFLICT upsert)。
// P0-D2(2026-08-21 乐观锁):ON CONFLICT DO UPDATE 加 WHERE user_vocabulary.version=$old,
// 旧版本冲突(并发 review)返 affected=0 行,调用方据此重读重算防 lost update。
// 返回 affected rows(0=版本冲突需重试,1=成功)。
func (d *VocabDAO) UpsertUserVocab(ctx context.Context, u *UserVocabRow) (int64, error) {
	res, err := d.pool.WriteDB().ExecContext(ctx,
		`INSERT INTO user_vocabulary
		   (user_id, word_id, srs_stage, interval_days, ease_factor, due_at,
		    review_count, lapse_count, status, last_reviewed, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0)
		 ON CONFLICT (user_id, word_id) DO UPDATE SET
		   srs_stage=EXCLUDED.srs_stage,
		   interval_days=EXCLUDED.interval_days,
		   ease_factor=EXCLUDED.ease_factor,
		   due_at=EXCLUDED.due_at,
		   review_count=EXCLUDED.review_count,
		   lapse_count=EXCLUDED.lapse_count,
		   status=EXCLUDED.status,
		   last_reviewed=EXCLUDED.last_reviewed,
		   version=user_vocabulary.version+1
		 WHERE user_vocabulary.version = $11`,
		u.UserID, u.WordID, u.SrsStage, u.IntervalDays, u.EaseFactor, u.DueAt,
		u.ReviewCount, u.LapseCount, u.Status, u.LastReviewed, u.Version)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListDueWords 查询某用户当前到期的词列表(连表 vocabulary 取词面信息),limit 控制数量。
func (d *VocabDAO) ListDueWords(ctx context.Context, userID int64, now time.Time, limit int) ([]VocabRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.pool.ReadDB().QueryContext(ctx,
		`SELECT v.id, v.word, v.phonetic, v.pos, v.definition, v.definition_zh, v.examples_json, v.cefr_level
		 FROM user_vocabulary uv
		 JOIN vocabulary v ON v.id = uv.word_id
		 WHERE uv.user_id = $1 AND uv.due_at <= $2
		 ORDER BY uv.due_at ASC
		 LIMIT $3`, userID, now.UTC(), limit)
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
	_, err := d.pool.WriteDB().ExecContext(ctx, `DELETE FROM vocabulary WHERE id = $1`, id)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
