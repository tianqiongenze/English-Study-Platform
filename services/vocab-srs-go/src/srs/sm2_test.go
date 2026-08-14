package sm2

import (
	"math"
	"testing"
	"time"
)

// 固定基准时刻,避免时区/时钟漂移影响断言。
var baseTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// expectedEase 计算标准 SM-2 ease_factor 更新(未 clamp),用于校验。
func expectedEase(ef float64, q int) float64 {
	return ef + (0.1 - float64(5-q)*(0.08+float64(5-q)*0.02))
}

func TestCompute_NewCardFirstReview_Perfect(t *testing.T) {
	// 新卡(q=5):reviewCount 0->1,interval=1,ease 略升,status learning->review
	in := Input{Quality: 5, IntervalDays: 1, EaseFactor: DefaultEase, ReviewCount: 0, Status: StatusLearning}
	r := Compute(in, baseTime)

	if r.IntervalDays != 1 {
		t.Errorf("interval: got %d, want 1", r.IntervalDays)
	}
	if r.ReviewCount != 1 {
		t.Errorf("reviewCount: got %d, want 1", r.ReviewCount)
	}
	if r.Status != StatusReview {
		t.Errorf("status: got %s, want %s", r.Status, StatusReview)
	}
	// q=5: delta=+0.1 -> 2.6,但 clamp 到 MaxEaseFactor=2.5
	wantEF := MaxEaseFactor
	if r.EaseFactor != wantEF {
		t.Errorf("easeFactor: got %.2f, want %.2f", r.EaseFactor, wantEF)
	}
	// 到期 = baseTime + 1 天
	wantDue := baseTime.AddDate(0, 0, 1).UTC()
	if !r.DueAt.Equal(wantDue) {
		t.Errorf("dueAt: got %v, want %v", r.DueAt, wantDue)
	}
}

func TestCompute_SecondReview_Interval6(t *testing.T) {
	// 第二次成功复习:interval 应为 6
	in := Input{Quality: 4, IntervalDays: 1, EaseFactor: 2.5, ReviewCount: 1, Status: StatusReview}
	r := Compute(in, baseTime)
	if r.IntervalDays != 6 {
		t.Errorf("interval: got %d, want 6", r.IntervalDays)
	}
	if r.ReviewCount != 2 {
		t.Errorf("reviewCount: got %d, want 2", r.ReviewCount)
	}
}

func TestCompute_ThirdReview_IntervalMultipliedByEF(t *testing.T) {
	// 第三次及以后:interval = round(prev_interval * ease_factor)
	prevInterval := 6
	ef := 2.5
	in := Input{Quality: 4, IntervalDays: prevInterval, EaseFactor: ef, ReviewCount: 2, Status: StatusReview}
	r := Compute(in, baseTime)
	want := int(math.Round(float64(prevInterval) * math.Round(expectedEase(ef, 4)*100) / 100))
	if r.IntervalDays != want {
		t.Errorf("interval: got %d, want %d", r.IntervalDays, want)
	}
	if r.ReviewCount != 3 {
		t.Errorf("reviewCount: got %d, want 3", r.ReviewCount)
	}
}

func TestCompute_Lapse_QualityBelow3_Resets(t *testing.T) {
	// q < 3:遗忘,interval 重置 1,reviewCount 归零,lapseCount++,status 回 learning
	in := Input{Quality: 2, IntervalDays: 30, EaseFactor: 2.2, ReviewCount: 5, LapseCount: 1, Status: StatusReview}
	r := Compute(in, baseTime)

	if r.IntervalDays != 1 {
		t.Errorf("interval: got %d, want 1 (reset)", r.IntervalDays)
	}
	if r.ReviewCount != 0 {
		t.Errorf("reviewCount: got %d, want 0 (reset)", r.ReviewCount)
	}
	if r.LapseCount != 2 {
		t.Errorf("lapseCount: got %d, want 2", r.LapseCount)
	}
	if r.Status != StatusLearning {
		t.Errorf("status: got %s, want %s", r.Status, StatusLearning)
	}
}

func TestCompute_Quality0_FullForget_EaseDrops(t *testing.T) {
	// q=0:ease_factor 大幅下降
	in := Input{Quality: 0, IntervalDays: 10, EaseFactor: 2.5, ReviewCount: 3, Status: StatusReview}
	r := Compute(in, baseTime)
	wantEF := math.Round(expectedEase(2.5, 0)*100) / 100
	if r.EaseFactor != wantEF {
		t.Errorf("easeFactor: got %.2f, want %.2f", r.EaseFactor, wantEF)
	}
	// q=0 => 2.5 + (0.1 - 5*(0.08+5*0.02)) = 2.5 + (0.1 - 0.9) = 1.7
	if r.EaseFactor < MinEaseFactor {
		t.Errorf("easeFactor below floor: %.2f", r.EaseFactor)
	}
}

func TestCompute_EaseFactorClampedToMin(t *testing.T) {
	// 连续低质量答错使 ease 跌破 1.3,应 clamp 到 1.3
	in := Input{Quality: 0, IntervalDays: 1, EaseFactor: MinEaseFactor, ReviewCount: 0, Status: StatusReview}
	r := Compute(in, baseTime)
	if r.EaseFactor < MinEaseFactor {
		t.Errorf("easeFactor below floor: got %.4f, want >= %.1f", r.EaseFactor, MinEaseFactor)
	}
}

func TestCompute_EaseFactorClampedToMax(t *testing.T) {
	// ef 输入非法(>2.5)应被重置为 DefaultEase=2.5
	in := Input{Quality: 5, IntervalDays: 1, EaseFactor: 3.0, ReviewCount: 0, Status: StatusLearning}
	r := Compute(in, baseTime)
	if r.EaseFactor > MaxEaseFactor {
		t.Errorf("easeFactor above ceiling: got %.4f, want <= %.1f", r.EaseFactor, MaxEaseFactor)
	}
}

func TestCompute_InvalidQuality_Clamped(t *testing.T) {
	// quality 超上限按 5 处理;超下限按 0 处理
	// 上限
	in := Input{Quality: 99, IntervalDays: 1, EaseFactor: 2.5, ReviewCount: 0, Status: StatusLearning}
	rHi := Compute(in, baseTime)
	if rHi.ReviewCount != 1 {
		t.Errorf("quality=99 clamped to 5: reviewCount got %d, want 1", rHi.ReviewCount)
	}
	// 下限
	in.Quality = -5
	rLo := Compute(in, baseTime)
	if rLo.ReviewCount != 0 {
		t.Errorf("quality=-5 clamped to 0: reviewCount got %d, want 0 (lapse)", rLo.ReviewCount)
	}
}

func TestCompute_MasteredPromotion(t *testing.T) {
	// interval>=21 且 reviewCount>=3 => status=mastered
	// 用 prevInterval=10, ef=2.5, q=5 => new ef≈2.5, interval=round(10*2.5)=25>=21, reviewCount=3
	in := Input{Quality: 5, IntervalDays: 10, EaseFactor: 2.5, ReviewCount: 2, Status: StatusReview}
	r := Compute(in, baseTime)
	if r.IntervalDays < MasteredMinInterval {
		t.Fatalf("prereq: interval %d < %d, adjust test", r.IntervalDays, MasteredMinInterval)
	}
	if r.ReviewCount < MasteredMinReviewCount {
		t.Fatalf("prereq: reviewCount %d < %d", r.ReviewCount, MasteredMinReviewCount)
	}
	if r.Status != StatusMastered {
		t.Errorf("status: got %s, want %s", r.Status, StatusMastered)
	}
}

func TestCompute_MasteredNotPromoted_IntervalTooSmall(t *testing.T) {
	// reviewCount 达标但 interval < 21:不升 mastered
	in := Input{Quality: 5, IntervalDays: 1, EaseFactor: 2.5, ReviewCount: 0, Status: StatusLearning}
	r := Compute(in, baseTime) // interval=1, reviewCount=1
	if r.Status == StatusMastered {
		t.Errorf("should not promote with interval=%d", r.IntervalDays)
	}
}

func TestCompute_LapseResetThreshold_FallsBackToLearning(t *testing.T) {
	// lapseCount 达 8:即使成功回忆也回退 learning
	in := Input{Quality: 5, IntervalDays: 30, EaseFactor: 2.5, ReviewCount: 5, LapseCount: LapseResetThreshold, Status: StatusReview}
	r := Compute(in, baseTime)
	if r.Status != StatusLearning {
		t.Errorf("lapse=%d should reset to learning, got %s", LapseResetThreshold, r.Status)
	}
}

func TestCompute_EmptyStatusDefaultsToLearning(t *testing.T) {
	in := Input{Quality: 0, IntervalDays: 1, EaseFactor: 2.5, ReviewCount: 0, Status: ""}
	r := Compute(in, baseTime)
	if r.Status != StatusLearning {
		t.Errorf("empty status should default to learning, got %s", r.Status)
	}
}

func TestCompute_DueAtCalculation(t *testing.T) {
	in := Input{Quality: 4, IntervalDays: 1, EaseFactor: 2.5, ReviewCount: 1, Status: StatusReview}
	r := Compute(in, baseTime)
	wantDue := baseTime.AddDate(0, 0, 6).UTC()
	if !r.DueAt.Equal(wantDue) {
		t.Errorf("dueAt: got %v, want %v", r.DueAt, wantDue)
	}
}

func TestRound2(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{2.555, 2.56},
		{2.554, 2.55},
		{1.3, 1.3},
		{0, 0},
		{2.5, 2.5},
	}
	for _, c := range cases {
		if got := round2(c.in); got != c.want {
			t.Errorf("round2(%.4f) = %.2f, want %.2f", c.in, got, c.want)
		}
	}
}
