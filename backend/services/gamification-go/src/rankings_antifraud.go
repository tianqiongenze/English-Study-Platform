// rankings_antifraud.go —— 反刷榜增强(偏差⑥):频率/设备指纹/可信时间窗/行为序列。
// 设计依据:docs/design/architecture/phase3-rankings-design.md §8 偏差⑥(P3 分阶段引入)。
// 现状反刷仅日 XP cap(dailyXPCap=500)+ 单事件 XP cap(maxEventXP=20),偏弱;本模块叠加:
//   ① 频率异常:短窗(60s)内同一用户事件数超 freqWindowCap → 本次事件降级(仅记 progress 不计 XP)。
//   ② 可信时间窗:payload 带客户端 ts 时,与服务端 now 偏差超 tsToleranceSec → 降级。
//   ③ 行为序列合理性:完课类事件(session_completed)需 payload 带 duration_sec,低于 minSessionSec → 降级。
//   ④ 设备指纹:payload 带 device_id 时,多账号对一设备检测(同 device 近 24h 关联 ≥3 uid → 该设备后续事件降级)。
// 降级策略统一为 "progress 保留、XP 清零",避免误杀正常用户(宁可放过不误扣)。
// risk-score 离线表(P3 后续)DEFERRED,本文件只做在线轻量检测。
package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

const (
	freqWindowCap    = 20   // 60s 内同一用户最多 20 个事件(超则降级)
	freqWindowSec    = 60
	tsToleranceSec   = 300 // 客户端 ts 与服务端偏差容忍 5min
	minSessionSec    = 30  // session_completed 时长下限
	dupDeviceUIDCap  = 3   // 同 device 24h 关联 uid 数上限
	dupDeviceWindow  = 24 * time.Hour
)

// antifraudDecision:是否计 XP。progress 始终保留(降级只清 XP)。
// 返回 reason 空串=通过,非空=降级原因(记日志,便于调阈值)。
func (s *Server) antifraudDecision(ctx context.Context, e learningEvent, payload map[string]any) (allowXP bool, reason string) {
	allowXP = true
	uid := e.UserID

	// ① 频率异常:60s 滑窗计数(IncrBy + EXPIRE)
	if r := s.rdb; r != nil {
		freqKey := "esp:gamif:freq:" + strconv.FormatInt(uid, 10)
		n, err := r.Incr(ctx, freqKey).Result()
		if err == nil {
			if n == 1 {
				r.Expire(ctx, freqKey, time.Duration(freqWindowSec)*time.Second)
			}
			if n > int64(freqWindowCap) {
				return false, "freq_window_exceeded"
			}
		}
	}

	// ② 可信时间窗:payload.client_ts(秒或毫秒)
	if payload != nil {
		if cts, ok := payload["client_ts"]; ok {
			var sec int64
			switch v := cts.(type) {
			case float64:
				sec = int64(v)
				if sec > 1e12 { // 毫秒
					sec /= 1000
				}
			case string:
				sec, _ = strconv.ParseInt(v, 10, 64)
			}
			if sec > 0 {
				delta := time.Now().Unix() - sec
				if delta < 0 {
					delta = -delta
				}
				if delta > tsToleranceSec {
					return false, "client_ts_out_of_window"
				}
			}
		}
	}

	// ③ 行为序列:session_completed 需合理时长
	if e.EventType == "session_completed" && payload != nil {
		durOK := false
		if d, ok := payload["duration_sec"].(float64); ok && d >= minSessionSec {
			durOK = true
		}
		if !durOK {
			return false, "session_duration_too_short"
		}
	}

	// ④ 设备指纹:多账号对一设备
	if payload != nil {
		if dev, ok := payload["device_id"].(string); ok && dev != "" {
			if r := s.rdb; r != nil {
				dkey := "esp:gamif:dev:" + dev
				added, err := r.SAdd(ctx, dkey, uid).Result()
				if err == nil && added > 0 {
					r.Expire(ctx, dkey, dupDeviceWindow)
				}
				cnt, _ := r.SCard(ctx, dkey).Result()
				if cnt > int64(dupDeviceUIDCap) {
					return false, "device_multi_account"
				}
			}
		}
	}

	return allowXP, ""
}

// suppressXPFromPayload 供 fanout 降级后清零 XP 用(progress 不变)。
// 保留签名以备未来 risk-score 表接入时复用。
func suppressXPFromPayload(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	if s, ok := payload["suppress_xp"].(bool); ok {
		return s
	}
	if s, ok := payload["suppress_xp"].(string); ok && s == "true" {
		return true
	}
	return false
}

var _ = json.Marshal // 预留:未来 risk-score 序列化用
