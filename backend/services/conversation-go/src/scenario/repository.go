// Package scenario 提供场景仓库:启动时从 CRDB 加载场景(读池),回退到内存默认场景。
// 数据访问统一走 esp-common-go/db 全局池(#140 全服务切 CRDB)。
package scenario

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	"esp/common-go/db"
)

// Scenario 是一个口语练习场景。
type Scenario struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	System     string `json:"system_prompt"`
	Difficulty int    `json:"difficulty"`
}

// Repository 场景仓库。
type Repository struct {
	mu     sync.RWMutex
	byCode map[string]Scenario
}

// NewRepository 返回含预置场景的仓库;若 CRDB 可用则从读池加载场景覆盖默认值。
func NewRepository() *Repository {
	r := &Repository{byCode: make(map[string]Scenario)}
	for _, s := range defaultScenarios() {
		r.byCode[s.Code] = s
	}
	// 从 CRDB 读池加载场景;DB 不可用时回退到内存默认值
	r.loadFromDB()
	return r
}

// loadFromDB 从 CRDB 读 scenarios 表,覆盖内存默认场景(只读,失败不阻断)。
func (r *Repository) loadFromDB() {
	ctx := context.Background()
	rows, err := db.Read(ctx, "SELECT code, title, system_prompt, difficulty FROM scenarios", false)
	if err != nil {
		slog.Warn("scenario: load from CRDB read pool failed, using defaults", "err", err)
		return
	}
	if rows == nil {
		return // DB_ENABLED=false,no-op
	}
	defer rows.Close()
	count := 0
	r.mu.Lock()
	for rows.Next() {
		var s Scenario
		if err := rows.Scan(&s.Code, &s.Title, &s.System, &s.Difficulty); err != nil {
			slog.Warn("scenario: scan row failed", "err", err)
			continue
		}
		r.byCode[s.Code] = s
		count++
	}
	r.mu.Unlock()
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		slog.Warn("scenario: rows iteration warning", "err", err)
	}
	slog.Info("scenario: loaded from CRDB", "count", count)
}

// List 返回全部场景。
func (r *Repository) List() []Scenario {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Scenario, 0, len(r.byCode))
	for _, s := range r.byCode {
		out = append(out, s)
	}
	return out
}

// Get 按 code 取场景。
func (r *Repository) Get(code string) (Scenario, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byCode[code]
	return s, ok
}

// Add 新增/覆盖场景(自定义场景)。
func (r *Repository) Add(s Scenario) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byCode[s.Code] = s
}

func defaultScenarios() []Scenario {
	return []Scenario{
		{
			Code: "airport", Title: "Airport Check-in", Difficulty: 3,
			System: "You are an airport check-in agent. Help the user check in for their flight. Be polite, ask for passport and booking reference, offer seat choices. Correct the user's English gently and provide rephrasing when needed.",
		},
		{
			Code: "interview", Title: "Job Interview", Difficulty: 4,
			System: "You are an interviewer for a tech company. Ask the user about their experience, strengths, and projects. Be professional. Gently correct grammar and suggest more native expressions.",
		},
		{
			Code: "cafe", Title: "Coffee Shop Ordering", Difficulty: 2,
			System: "You are a friendly barista. Help the user order coffee. Suggest seasonal drinks, ask about size and milk options. Correct mistakes kindly and offer rephrasing.",
		},
	}
}
