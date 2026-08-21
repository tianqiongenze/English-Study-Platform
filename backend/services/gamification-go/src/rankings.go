// rankings.go —— 排行榜(功能拓展 Phase3):XP + 进度双榜,按 global/region/circle/category 扇出。
// 设计依据:docs/design/architecture/phase3-rankings-design.md(SDD+DDD+TOGAF+读写分离+L1L2L3 多级缓存)。
// 喂数源 = 轮询 esp_learning.learning_events(id 游标,自部署时 MAX(id) 起,避免历史灌数据 flood)。
//   Pulsar-only 会漏 content 事件(content 经 outbox→OutboxRelay 直插 learning_events,不走 Pulsar),
//   故以落库事件表为唯一完整源,2s 近实时,幂等(id 高水位 + message_id SETNX 双保险,偏差⑤),多副本经 Redis 分布式锁防重复计分。
// 反刷榜:单事件 XP 上限 + 每日 XP 上限(daily cap);opt-out 写端拦截(偏差②:不在读端过滤,直接不扇出)。
// 平局:复合 score(偏差③)xp*1e10 - ts_offset,XP 相同早达成优先;读榜还原 XP。
// 快照:每日桶翻转时把当日全量写 CRDB leaderboard_snapshots(偏差④:支持 percentile)。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	feedPollInterval = 2 * time.Second
	feedBatchSize    = 200
	feedHwmKey       = "esp:gamif:feed:hwm"
	feedLockKey      = "esp:gamif:feed:lock"
	feedLockTTL      = 10 * time.Second
	dayScopesKey     = "esp:gamif:day:%s:scopes" // SADD 记录当日已写 ZSET key,日切快照用
	dailyXPCap       = 500                       // 每日 XP 上限(反刷)
	maxEventXP       = 20                        // 单事件 XP 上限(反刷)
	snapshotTopN     = 100
)

// 事件类型 → 基础 XP(可扩展;未知事件给 1)。
var baseXP = map[string]float64{
	"answer":              5,
	"practice_attempt":    3,
	"content.item_answered": 5,
	"session_completed":    10,
	"speech.completed":     8,
	"writing.submitted":    10,
}

type userInfo struct {
	region string
	optOut bool
}

var (
	userCacheMu sync.RWMutex
	userCache   = map[int64]userInfo{}
	userCacheTS = map[int64]time.Time{}
)

type learningEvent struct {
	ID        int64
	UserID    int64
	EventType string
	Payload   []byte
}

// ---- 周期桶 ----

func periodBucket(now time.Time, period string) string {
	switch period {
	case "daily":
		return now.Format("20060102")
	case "monthly":
		return now.Format("200601")
	default: // weekly
		_, wk := now.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", now.Year(), wk)
	}
}

func lbKey(typ, scope, scopeID, period string, now time.Time) string {
	b := periodBucket(now, period)
	return fmt.Sprintf("esp:gamif:lb:%s:%s:%s:%s", typ, scope, scopeID, b)
}

// ---- XP / 进度 推导 ----

// xpFromEvent 返回 (xp, progress)。过滤无关事件(emotion/探针)。
func xpFromEvent(typ string, payload map[string]any) (float64, float64) {
	if typ == "emotion" || strings.HasPrefix(typ, "crdb.") || typ == "" {
		return 0, 0
	}
	correct := false
	if payload != nil {
		if v, ok := payload["correct"].(bool); ok {
			correct = v
		} else if s, ok := payload["correct"].(string); ok && s == "true" {
			correct = true
		}
		if a, ok := payload["answer"].(string); ok && a == "correct" { // practice_attempt
			correct = true
		}
	}
	xp := baseXP[typ]
	if xp <= 0 {
		xp = 1
	}
	switch typ {
	case "answer":
		if correct {
			xp = 8
		} else {
			xp = 2
		}
	case "practice_attempt":
		if !correct {
			xp = 1
		}
	case "content.item_answered":
		if correct {
			xp = 5
		} else {
			xp = 1
		}
		if payload != nil {
			if s, _ := payload["score"].(float64); s >= 100 {
				xp += 2
			}
		}
	}
	if xp > maxEventXP {
		xp = maxEventXP
	}
	progress := 0.0
	if xp >= 3 {
		progress = 1 // 判定为有效学习动作
	}
	return xp, progress
}

// categoryOf:content 事件 payload 带 category/item_type;其余归 general。
func categoryOf(typ string, payload map[string]any) string {
	if payload != nil {
		if c, ok := payload["category"].(string); ok && c != "" {
			return c
		}
		if c, ok := payload["item_type"].(string); ok && c != "" {
			return c
		}
	}
	if idx := strings.IndexByte(typ, '.'); idx > 0 {
		return strings.ToLower(typ[:idx])
	}
	return strings.ToLower(typ)
}

// regionAncestors:region_code 如 CN/Guangdong/Shenzhen → 各级祖先前缀。
func regionAncestors(region string) []string {
	var out []string
	cur := ""
	for _, p := range strings.Split(region, "/") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if cur == "" {
			cur = p
		} else {
			cur = cur + "/" + p
		}
		out = append(out, cur)
	}
	return out
}

// ---- 用户信息缓存(region/opt-out)- ----

func (s *Server) loadUsers(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	var fresh []int64
	userCacheMu.RLock()
	for _, id := range ids {
		if _, ok := userCache[id]; !ok || now.Sub(userCacheTS[id]) > 5*time.Minute {
			fresh = append(fresh, id)
		}
	}
	userCacheMu.RUnlock()
	if len(fresh) == 0 {
		return nil
	}
	ph := make([]string, len(fresh))
	args := make([]any, len(fresh))
	for i, id := range fresh {
		ph[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}
	rows, err := s.readDB.QueryContext(ctx,
		"SELECT id, COALESCE(region_code,''), ranking_opt_out FROM esp_auth.users WHERE id IN ("+strings.Join(ph, ",")+")",
		args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	userCacheMu.Lock()
	defer userCacheMu.Unlock()
	for rows.Next() {
		var id int64
		var region string
		var opt bool
		if err := rows.Scan(&id, &region, &opt); err != nil {
			continue
		}
		userCache[id] = userInfo{region: region, optOut: opt}
		userCacheTS[id] = now
	}
	return rows.Err()
}

// ---- 每日 XP 上限(cap 反刷)- ----

func (s *Server) dailyUnderCap(ctx context.Context, uid int64, xp float64, day string) bool {
	key := fmt.Sprintf("esp:gamif:xpc:%d:%s", uid, day)
	total, err := s.rdb.IncrBy(ctx, key, int64(xp)).Result()
	if err != nil {
		return true // Redis 异常不阻断(退让给 ZSET 反刷上限兜底)
	}
	return total <= dailyXPCap
}

// ---- 扇出 ----

// compositeScore(偏差③):复合 score = xp*1e10 - ts_offset。
// ts_offset = 事件相对部署纪元(2026-01-01)的纳秒,值越大(越晚)扣分越多 → XP 相同时早达成者优先。
// 读榜时还原 XP:realXP = round((score + ts_offset)/1e10) ≈ round(score/1e10)(ts_offset 远小于 xp*1e10 的低 10 位影响)。
const (
	scoreScale   = 1e10
	scoreEpochNS = int64(1735689600_000_000_000) // 2026-01-01T00:00:00Z 纳秒
)

func compositeScore(xp float64, now time.Time) float64 {
	ts := now.UnixNano() - scoreEpochNS
	if ts < 0 {
		ts = 0
	}
	return xp*scoreScale - float64(ts%int64(scoreScale))
}

// xpFromScore 近似还原 XP(读榜展示用;丢 ts_offset 低位)。
func xpFromScore(s float64) float64 {
	if s <= 0 {
		return 0
	}
	return s / scoreScale
}

func (s *Server) fanout(ctx context.Context, e learningEvent, info userInfo, circles []int64) {
	if info.optOut {
		return // 偏差②:opt-out 写端拦截,不入任何 ZSET(读端不再每请求过滤)
	}
	payload := map[string]any{}
	if len(e.Payload) > 0 {
		_ = json.Unmarshal(e.Payload, &payload)
	}
	xp, progress := xpFromEvent(e.EventType, payload)
	if xp <= 0 && progress <= 0 {
		return
	}
	// 偏差⑥:反刷增强。降级只清 XP(progress 保留),宁可放过不误杀。
	if xp > 0 {
		if allow, reason := s.antifraudDecision(ctx, e, payload); !allow {
			log.Printf("[%s] antifraud downgrade uid=%d event=%s reason=%s (XP cleared, progress kept)", serviceName, e.UserID, e.EventType, reason)
			xp = 0
		}
	}
	now := time.Now()
	day := now.Format("20060102")
	cat := categoryOf(e.EventType, payload)
	if xp > 0 && !s.dailyUnderCap(ctx, e.UserID, xp, day) {
		return // 当日 XP 已达上限,本次不再入榜(XP 已降级为 0 则跳过 cap 检查)
	}
	regions := regionAncestors(info.region)
	// 偏差③:XP 用复合 score;进度(prog)无序性强,保留原值(进度同值意义弱,不强制 tie-break)。
	xpScore := compositeScore(xp, now)

	pipe := s.rdb.Pipeline()
	scopeAdd := func(scope, sid string) {
		for _, typ := range []string{"xp", "prog"} {
			key := lbKey(typ, scope, sid, "daily", now)
			if typ == "xp" {
				pipe.ZIncrBy(ctx, key, xpScore, uidStr(e.UserID))
			} else {
				pipe.ZIncrBy(ctx, key, progress, uidStr(e.UserID))
			}
			pipe.SAdd(ctx, fmt.Sprintf(dayScopesKey, day), key)
			// weekly/monthly 复用每日累加?为独立周期,直接同写入周/月桶
		}
		// 周/月桶(独立 ZSET,周期翻转自然失效)
		for _, period := range []string{"weekly", "monthly"} {
			for _, typ := range []string{"xp", "prog"} {
				key := lbKey(typ, scope, sid, period, now)
				score := progress
				if typ == "xp" {
					score = xpScore
				}
				pipe.ZIncrBy(ctx, key, score, uidStr(e.UserID))
			}
		}
	}
	scopeAdd("global", "all")
	for _, rc := range regions {
		scopeAdd("region", rc)
	}
	for _, cid := range circles {
		scopeAdd("circle", strconv.FormatInt(cid, 10))
	}
	if cat != "" {
		scopeAdd("category", cat)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[%s] fanout pipe err: %v", serviceName, err)
	}
	// 失效该日榜 L1 top-N 缓存(下次读榜重算)
	rankCacheEvict(lbKey("xp", "global", "all", "daily", now))
}

// ---- 轮询喂数 ----

// message_idFromPayload 从 learning_events.payload(JSONB)取 message_id;无则用 id 兜底(老事件)。
// 偏差⑤:幂等去重键。content 事件经 outbox relay 带 message_id;非 content 事件无则退化为 id(仍唯一)。
func message_idFromPayload(id int64, payload []byte) string {
	if len(payload) > 0 {
		var p struct {
			MessageID string `json:"message_id"`
			MsgID     string `json:"msg_id"`
		}
		if json.Unmarshal(payload, &p) == nil {
			if p.MessageID != "" {
				return p.MessageID
			}
			if p.MsgID != "" {
				return p.MsgID
			}
		}
	}
	return "id:" + strconv.FormatInt(id, 10)
}

// processedMark:偏差⑤ SETNX 幂等去重。已处理返回 false(重复),首次返回 true。
// key = esp:gamif:processed:{msg_id},NX EX 86400(1 天,覆盖日切重放窗口)。
func (s *Server) processedMark(ctx context.Context, msgID string) bool {
	if msgID == "" {
		return true
	}
	ok, err := s.rdb.SetNX(ctx, "esp:gamif:processed:"+msgID, "1", 24*time.Hour).Result()
	if err != nil {
		return true // Redis 异常不阻断(退让给 hwm + 分布式锁兜底),仅记日志
	}
	return ok
}

func (s *Server) pollLoop() {
	ctx := context.Background()
	// 高水位:Redis 持久(重启/多副本共享)优先;首次无值时从当前 MAX(id) 开始,忽略历史灌数据。
	var hwm int64
	if rv, err := s.rdb.Get(ctx, feedHwmKey).Result(); err == nil && rv != "" {
		if v, perr := strconv.ParseInt(rv, 10, 64); perr == nil {
			hwm = v
			log.Printf("[%s] rankings feed resume hwm=%d (from Redis)", serviceName, hwm)
		}
	}
	if hwm == 0 {
		if err := s.readDB.QueryRowContext(ctx,
			"SELECT COALESCE(MAX(id),0) FROM esp_learning.learning_events").Scan(&hwm); err == nil {
			log.Printf("[%s] rankings feed init hwm=%d (start from now, ignore historical data)", serviceName, hwm)
		}
	}
	lastDaily := ""
	ticker := time.NewTicker(feedPollInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		day := now.Format("20060102")
		if lastDaily != "" && lastDaily != day {
			go s.snapshotDay(ctx, lastDaily) // 日切:上一日快照入库
		}
		lastDaily = day

		// 多副本互斥:抢锁失败说明别的副本在消费,跳过本批(防重复计分)
		locked, err := s.rdb.SetNX(ctx, feedLockKey, uidStr(time.Now().UnixNano()), feedLockTTL).Result()
		if err != nil || !locked {
			continue
		}
		events, newHwm, err := s.fetchEvents(ctx, hwm)
		if err != nil {
			_ = s.rdb.Del(ctx, feedLockKey).Err()
			continue
		}
		if len(events) > 0 {
			ids := make([]int64, 0, len(events))
			for _, e := range events {
				ids = append(ids, e.UserID)
			}
			if err := s.loadUsers(ctx, ids); err != nil {
				// 用户元数据查询失败:不推进 hwm,下个 tick 重试本批(避免事件永久丢失)
				log.Printf("[%s] loadUsers err retry batch: %v", serviceName, err)
				_ = s.rdb.Del(ctx, feedLockKey).Err()
				continue
			}
			circles, cerr := s.memberCircleMap(ctx, ids)
			if cerr != nil {
				log.Printf("[%s] memberCircleMap err: %v", serviceName, cerr)
			}
			// P1-D1(2026-08-21 EER):hwm 推进策略修正,防事件计分永久丢失。
			// 原:processedMark 未命中或 userCache 未命中时 continue 跳过,但 hwm=newHwm 无条件推进
			// → 被跳过事件计分永久丢失。现分两类:
			// ① processedMark 已处理(重复消费/重放)→ 跳过合理(幂等去重,已处理过不丢),允许 hwm 推进;
			// ② userCache 未命中(loadUsers 成功但该 user 不在 cache)→ 真 Gap,不推进 hwm 整批重试,
			//    避免丢失该事件计分(下个 tick 重新 fetch 同批重新 loadUsers,多数为 loadUsers 暂态抖动自愈)。
			missingUser := false // 本批是否有 userCache 未命中事件(真 Gap)
			for _, e := range events {
				msgID := message_idFromPayload(e.ID, e.Payload)
				if !s.processedMark(ctx, msgID) {
					continue // 已处理过(幂等去重),跳过合理,hwm 可推进
				}
				userCacheMu.RLock()
				info, ok := userCache[e.UserID]
				userCacheMu.RUnlock()
				if !ok {
					missingUser = true
					log.Printf("[%s] pollLoop userCache miss uid=%d eid=%d — skip hwm advance, retry batch", serviceName, e.UserID, e.ID)
					continue // 未命中:不推进 hwm,整批下个 tick 重试(防计分丢失)
				}
				s.fanout(ctx, e, info, circles[e.UserID])
			}
			if missingUser {
				// 有未命中事件:不推进 hwm,释放锁,下个 tick 重试本批(已 fanout 的事件由 processedMark 幂等防重复)
				_ = s.rdb.Del(ctx, feedLockKey).Err()
				continue
			}
			hwm = newHwm
			_ = s.rdb.Set(ctx, feedHwmKey, strconv.FormatInt(hwm, 10), 0).Err()
		}
		_ = s.rdb.Del(ctx, feedLockKey).Err()
	}
}

func (s *Server) fetchEvents(ctx context.Context, hwm int64) ([]learningEvent, int64, error) {
	rows, err := s.readDB.QueryContext(ctx,
		"SELECT id, user_id, event_type, payload FROM esp_learning.learning_events WHERE id > $1 ORDER BY id ASC LIMIT $2",
		hwm, feedBatchSize)
	if err != nil {
		return nil, hwm, err
	}
	defer rows.Close()
	var events []learningEvent
	var maxID int64
	for rows.Next() {
		var e learningEvent
		var b []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &b); err != nil {
			continue
		}
		e.Payload = b
		events = append(events, e)
		if e.ID > maxID {
			maxID = e.ID
		}
	}
	if maxID == 0 {
		return nil, hwm, nil
	}
	return events, maxID, nil
}

// memberCircleMap:一批用户 → 各自加入的圈子(esp_community 跨库)。
func (s *Server) memberCircleMap(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	if len(ids) == 0 {
		return map[int64][]int64{}, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}
	rows, err := s.readDB.QueryContext(ctx,
		"SELECT user_id, circle_id FROM esp_community.circle_memberships WHERE user_id IN ("+strings.Join(ph, ",")+")",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var uid, cid int64
		if err := rows.Scan(&uid, &cid); err != nil {
			continue
		}
		out[uid] = append(out[uid], cid)
	}
	return out, nil
}

// ---- 快照:上一日全量写 CRDB(偏差④:支持任意用户历史 percentile)----

// snapshotDay 日切把当日 daily 桶全量 rank/total 写 leaderboard_snapshots。
// 偏差④:不再只 top-100;全量 ZREVRANGE 0 -1,大榜分批(snapshotBatchSize)事务写。
// total 用 ZCARD 一次性取,与 rank 一同落库 → rankingGap percentile 可走快照非实时 ZCOUNT。
const snapshotBatchSize = 500

func (s *Server) snapshotDay(ctx context.Context, day string) {
	scopeKeys, err := s.rdb.SMembers(ctx, fmt.Sprintf(dayScopesKey, day)).Result()
	if err != nil || len(scopeKeys) == 0 {
		return
	}
	for _, typ := range []string{"xp", "prog"} {
		for _, key := range scopeKeys {
			parts := strings.Split(key, ":")
			if len(parts) != 7 || parts[3] != typ || parts[6] != day {
				continue
			}
			total, err := s.rdb.ZCard(ctx, key).Result()
			if err != nil || total == 0 {
				continue
			}
			// 全量分批拉取(避免大榜一次 ZREVRANGE 撑爆)
			var allMembers []redis.Z
			for start := int64(0); start < total; start += snapshotBatchSize {
				end := start + snapshotBatchSize - 1
				if end > total-1 {
					end = total - 1
				}
				ms, err := s.rdb.ZRevRangeWithScores(ctx, key, start, end).Result()
				if err != nil {
					break
				}
				allMembers = append(allMembers, ms...)
			}
			if len(allMembers) == 0 {
				continue
			}
			// 分批事务写(单事务过大 CRDB 会拒)
			for start := 0; start < len(allMembers); start += snapshotBatchSize {
				end := start + snapshotBatchSize
				if end > len(allMembers) {
					end = len(allMembers)
				}
				tx, err := s.writeDB.BeginTx(ctx, nil)
				if err != nil {
					return
				}
				for i := start; i < end; i++ {
					m := allMembers[i]
					uid, err := strconv.ParseInt(fmt.Sprint(m.Member), 10, 64)
					if err != nil {
						continue
					}
					if _, err := tx.ExecContext(ctx,
						"INSERT INTO esp_gamif.leaderboard_snapshots (scope_type, scope_id, period, rank, user_id, score, total_members) VALUES ($1,$2,$3,$4,$5,$6,$7)",
						parts[4], parts[5], "daily", i+1, uid, m.Score, total); err != nil {
						log.Printf("[%s] snapshot insert err: %v", serviceName, err)
					}
				}
				if err := tx.Commit(); err != nil {
					log.Printf("[%s] snapshot commit err: %v", serviceName, err)
				}
			}
		}
	}
	_ = s.rdb.Del(ctx, fmt.Sprintf(dayScopesKey, day))
}

// ---- HTTP:rankings ----

type rankRow struct {
	Rank   int     `json:"rank"`
	UserID int64   `json:"user_id"`
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
}

// rankings 读榜(偏差②:opt-out 已写端拦截,读端不再每请求查 DB;仅对历史遗留 opt-out 用户做 L1 缓存过滤)。
// 偏差③:XP 榜 score 为复合分,展示前用 xpFromScore 还原。多级缓存:L1 top-N(60s)→ L2 ZSET。
func (s *Server) rankings(c *gin.Context) {
	typ := c.DefaultQuery("type", "xp")
	scope := c.DefaultQuery("scope", "global")
	scopeID := c.DefaultQuery("scope_id", "all")
	period := c.DefaultQuery("period", "weekly")
	limit := parseTop(c.DefaultQuery("limit", "20")) + 1
	if limit > 100 {
		limit = 100
	}
	meStr := c.GetHeader("X-User-Id")
	key := lbKey(typ, scope, scopeID, period, time.Now())

	// L1 top-N 缓存命中则直接回(热榜降 ZREVRANGE 成本)。
	if cached, ok := rankCacheGet(c.Request.Context(), key); ok && int64(len(cached)) >= limit {
		rows := cached[:min(int(limit), len(cached))]
		mine := s.buildMine(c.Request.Context(), key, meStr, rows)
		c.JSON(200, gin.H{"list": rows, "as_of": time.Now().UTC().Format(time.RFC3339), "mine": mine})
		return
	}

	members, err := s.rdb.ZRevRangeWithScores(c.Request.Context(), key, 0, int64(limit+30)).Result()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	optOut := s.refreshedOptOut(c.Request.Context())

	var rows []rankRow
	for _, m := range members {
		uid, err := strconv.ParseInt(fmt.Sprint(m.Member), 10, 64)
		if err != nil {
			continue
		}
		if optOut[uid] {
			continue
		}
		score := m.Score
		if typ == "xp" {
			score = xpFromScore(score) // 偏差③:还原 XP
		}
		rows = append(rows, rankRow{Rank: len(rows) + 1, UserID: uid, Score: score})
		if int64(len(rows)) >= limit {
			break
		}
	}
	// 用户名校对
	names := s.userNameMap(c.Request.Context(), rows)
	for i := range rows {
		rows[i].Name = names[rows[i].UserID]
	}
	rankCacheSet(key, rows) // 回填 L1

	mine := s.buildMine(c.Request.Context(), key, meStr, rows)
	c.JSON(200, gin.H{"list": rows, "as_of": time.Now().UTC().Format(time.RFC3339), "mine": mine})
}

func (s *Server) buildMine(ctx context.Context, key, meStr string, rows []rankRow) gin.H {
	if meStr == "" {
		return gin.H{}
	}
	uid, err := strconv.ParseInt(meStr, 10, 64)
	if err != nil {
		return gin.H{}
	}
	optOut := s.refreshedOptOut(ctx)
	if optOut[uid] {
		return gin.H{}
	}
	rank := s.myRank(ctx, key, uid, optOut)
	score, _ := s.rdb.ZScore(ctx, key, meStr).Result()
	top, _ := s.rdb.ZRevRangeWithScores(ctx, key, 0, 0).Result()
	gap := float64(0)
	if len(top) > 0 {
		gap = xpFromScore(top[0].Score) - xpFromScore(score)
	}
	return gin.H{"user_id": uid, "rank": rank, "score": xpFromScore(score), "gap_to_top": gap}
}

func (s *Server) myRank(ctx context.Context, key string, uid int64, optOut map[int64]bool) int {
	// 线性扫描 rank(a) 之前的成员并减去已 opt-out 的(数据量小可接受)
	rank, err := s.rdb.ZRevRank(ctx, key, uidStr(uid)).Result()
	if err != nil || rank < 0 {
		return 0 // 未上榜(Redis 无该成员,ZRevRank 返回 Err 且 Val=0)
	}
	above, err := s.rdb.ZRevRangeWithScores(ctx, key, 0, rank).Result()
	if err != nil {
		return int(rank) + 1
	}
	adjusted := 0
	for _, m := range above {
		u, _ := strconv.ParseInt(fmt.Sprint(m.Member), 10, 64)
		if optOut[u] {
			continue
		}
		adjusted++
	}
	return adjusted
}

func (s *Server) refreshedOptOut(ctx context.Context) map[int64]bool {
	out := map[int64]bool{}
	rows, err := s.readDB.QueryContext(ctx,
		"SELECT id FROM esp_auth.users WHERE ranking_opt_out = true")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			out[id] = true
		}
	}
	return out
}

func (s *Server) userNameMap(ctx context.Context, rows []rankRow) map[int64]string {
	out := map[int64]string{}
	if len(rows) == 0 {
		return out
	}
	ph := make([]string, len(rows))
	args := make([]any, len(rows))
	for i, r := range rows {
		ph[i] = "$" + strconv.Itoa(i+1)
		args[i] = r.UserID
	}
	rs, err := s.readDB.QueryContext(ctx,
		"SELECT id, name FROM esp_auth.users WHERE id IN ("+strings.Join(ph, ",")+")", args...)
	if err != nil {
		return out
	}
	defer rs.Close()
	for rs.Next() {
		var id int64
		var name string
		if err := rs.Scan(&id, &name); err == nil {
			out[id] = name
		}
	}
	return out
}

// ---- HTTP:gap / regions / opt-out ----

func (s *Server) rankingGap(c *gin.Context) {
	typ := c.DefaultQuery("type", "xp")
	scope := c.DefaultQuery("scope", "global")
	scopeID := c.DefaultQuery("scope_id", "all")
	period := c.DefaultQuery("period", "weekly")
	meStr := c.GetHeader("X-User-Id")
	if meStr == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	key := lbKey(typ, scope, scopeID, period, time.Now())
	optOut := s.refreshedOptOut(c.Request.Context())
	uid, _ := strconv.ParseInt(meStr, 10, 64)
	if optOut[uid] {
		c.JSON(200, gin.H{"opted_out": true})
		return
	}
	rank := s.myRank(c.Request.Context(), key, uid, optOut)
	score, _ := s.rdb.ZScore(c.Request.Context(), key, meStr).Result()
	total := s.totalMembers(c.Request.Context(), key, optOut)
	top, _ := s.rdb.ZRevRangeWithScores(c.Request.Context(), key, 0, 1).Result()
	var gapTop, gapPrev float64
	if len(top) > 0 {
		gapTop = xpFromScore(top[0].Score) - xpFromScore(score)
	}
	if len(top) > 1 {
		gapPrev = xpFromScore(top[1].Score) - xpFromScore(score)
	}
	percentile := 0.0
	if total > 0 && rank > 0 {
		percentile = (1 - float64(rank-1)/float64(total)) * 100
	}
	c.JSON(200, gin.H{"my_rank": rank, "my_score": xpFromScore(score), "total": total,
		"gap_to_top": gapTop, "gap_to_prev": gapPrev, "percentile": percentile})
}

func (s *Server) totalMembers(ctx context.Context, key string, optOut map[int64]bool) int {
	cnt, err := s.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return 0
	}
	// ZCard 含 opt-out;为分摊成本,超出 1000 时近似(文档注明),否则精确过滤
	if cnt <= 1000 {
		all, _ := s.rdb.ZRange(ctx, key, 0, -1).Result()
		sub := 0
		for _, m := range all {
			if u, err := strconv.ParseInt(m, 10, 64); err == nil && optOut[u] {
				sub++
			}
		}
		return int(cnt) - sub
	}
	return int(cnt)
}

func (s *Server) regions(c *gin.Context) {
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		"SELECT DISTINCT region_code FROM esp_auth.users WHERE region_code IS NOT NULL AND region_code != '' ORDER BY region_code")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			out = append(out, gin.H{"code": code, "label": lastLabel(code)})
		}
	}
	c.JSON(200, out)
}

func lastLabel(code string) string {
	parts := strings.Split(code, "/")
	return parts[len(parts)-1]
}

// opt-out 切换(写 esp_auth.users + 即时反映:读端 refreshedOptOut 过滤)
func (s *Server) setOptOut(c *gin.Context) {
	meStr := c.GetHeader("X-User-Id")
	if meStr == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	uid, err := strconv.ParseInt(meStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "bad user"})
		return
	}
	var body struct {
		OptOut bool `json:"opt_out"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if _, err := s.writeDB.ExecContext(c.Request.Context(),
		"UPDATE esp_auth.users SET ranking_opt_out = $1 WHERE id = $2", body.OptOut, uid); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	userCacheMu.Lock()
	userCache[uid] = userInfo{region: userCache[uid].region, optOut: body.OptOut}
	userCacheTS[uid] = time.Now()
	userCacheMu.Unlock()
	c.JSON(200, gin.H{"opt_out": body.OptOut})
}

// ---- mics ----