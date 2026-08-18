// Package sm2 实现 SM-2+ 间隔重复算法(Anki 风格增强版)。
//
// 标准 SM-2 公式(SuperMemo 2):
//   ease_factor' = ease_factor + (0.1 - (5-q)*(0.08 + (5-q)*0.02))
//   ease_factor 范围 [1.3, 2.5]
//   若 q < 3:重复(lapse_count++),interval 重置为 1
//   否则:
//     review_count == 1 -> interval = 1
//     review_count == 2 -> interval = 6
//     review_count >= 3 -> interval = round(prev_interval * ease_factor)
//
// SM-2+ 增强:
//   - 新词进入 learning 阶段(interval=1, status=learning)
//   - 连续 3 次 review 且 interval >= 21 天 -> status=mastered
//   - lapse_count 达到 8 视为需要重新学习(不删除,标记 learning)
//   - q 映射:0=完全忘记 / 1=有印象但错 / 2=艰难回忆 / 3=正确但费力 / 4=正确略迟 / 5=完美
package sm2

import (
	"math"
	"time"
)

// 质量 q 的合法范围
const (
	MinQuality = 0
	MaxQuality = 5
)

// EaseFactor 边界(标准 SM-2 下限 1.3,Anki 上限 2.5)
const (
	MinEaseFactor = 1.3
	MaxEaseFactor = 2.5
	DefaultEase   = 2.5
)

// 状态常量(对齐 schema esp_vocab.user_vocabulary.status)
const (
	StatusLearning  = "learning"
	StatusReview    = "review"
	StatusMastered  = "mastered"
)

// 卡片转入 mastered 的阈值
const (
	MasteredMinInterval    = 21 // 天
	MasteredMinReviewCount = 3
	LapseResetThreshold    = 8  // lapse 达此值回退到 learning
)

// MaxInterval 封顶单次复习间隔上限(天)。
// 不设上限时 interval 按 prev*ef 指数增长,数千次复习后可达天文数字,
// due_at 超出 DB timestamp 范围(如 CRDB/PG 上限 294246 年)导致写库 500。
// 365 天封顶:mastered 卡片最长年复习一次,兼顾记忆曲线与存储安全。
const MaxInterval = 365

// Input 是计算 SM-2+ 一次复习结果所需的当前卡片状态。
type Input struct {
	Quality      int     // 0-5
	IntervalDays int     // 当前 interval(新词 1)
	EaseFactor   float64 // 当前 ease_factor(新词 2.5)
	ReviewCount  int     // 已成功复习次数(不含本次)
	LapseCount   int     // 已遗忘次数
	Status       string  // learning / review / mastered
}

// Result 是 SM-2+ 一次复习后输出的新卡片状态。
type Result struct {
	IntervalDays int
	EaseFactor   float64
	DueAt        time.Time // 当前时刻 + interval 天
	ReviewCount  int
	LapseCount   int
	Status       string
}

// Compute 执行一次 SM-2+ 调度计算。
// 输入非法 quality 时按 q=0(完全忘记)处理,保证幂等不崩。
func Compute(in Input, now time.Time) Result {
	q := in.Quality
	if q < MinQuality {
		q = MinQuality
	} else if q > MaxQuality {
		q = MaxQuality
	}

	ef := in.EaseFactor
	if ef < MinEaseFactor || ef > MaxEaseFactor {
		ef = DefaultEase
	}

	// 1. 更新 ease_factor(标准 SM-2 公式)
	ef = ef + (0.1 - float64(5-q)*(0.08+float64(5-q)*0.02))
	if ef < MinEaseFactor {
		ef = MinEaseFactor
	} else if ef > MaxEaseFactor {
		ef = MaxEaseFactor
	}

	// 2. 间隔计算
	reviewCount := in.ReviewCount
	lapseCount := in.LapseCount
	status := in.Status
	if status == "" {
		status = StatusLearning
	}

	var interval int
	if q < 3 {
		// 遗忘:重置 interval=1,lapse++
		interval = 1
		lapseCount++
		reviewCount = 0
		status = StatusLearning
	} else {
		// 成功回忆
		reviewCount++
		switch reviewCount {
		case 1:
			interval = 1
		case 2:
			interval = 6
		default:
			interval = int(math.Round(float64(in.IntervalDays) * ef))
			if interval < 1 {
				interval = 1
			}
			if interval > MaxInterval {
				interval = MaxInterval
			}
		}
		// 阶段升级
		if status == StatusLearning {
			status = StatusReview
		}
		if interval >= MasteredMinInterval && reviewCount >= MasteredMinReviewCount {
			status = StatusMastered
		}
	}

	// 3. 遗忘次数过多回退
	if lapseCount >= LapseResetThreshold && status != StatusLearning {
		status = StatusLearning
	}

	// 4. 计算到期时间(当前 + interval 天,UTC)
	dueAt := now.AddDate(0, 0, interval).UTC()

	return Result{
		IntervalDays: interval,
		EaseFactor:   round2(ef),
		DueAt:        dueAt,
		ReviewCount:  reviewCount,
		LapseCount:   lapseCount,
		Status:       status,
	}
}

// round2 保留两位小数(对齐 schema DECIMAL(4,2))。
func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
