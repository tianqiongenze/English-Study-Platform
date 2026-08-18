// gamification-go —— 学习目标/连胜/排行榜/成就/积分商城(S7, P1, 事件驱动)
// scaffold 级:健康检查 + 积分/连胜/排行榜/成就路由桩 + Pulsar 订阅 esp.business.*(桩)
// ADR: docs/design/adr-new-services.md / PRD §七 / schema esp_gamification
//
// 事件驱动:Pulsar 订阅 esp.business.* 主题。rankings_projector.go(pulsar_eda tag 启用)
// 订阅 learning.event.report 近实时投影排行榜;pollLoop 作完整源兜底。
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/common-go/redisx"
	"esp/common-go/registry"
)

const serviceName = "gamification-go"

// Server 持有依赖,所有 handler 通过方法绑定 -> 可注入 mock DB/Redis 做单测。
type Server struct {
	readDB  *sql.DB
	writeDB *sql.DB
	rdb     *redis.ClusterClient
}

func NewServer(readDB, writeDB *sql.DB, rdb *redis.ClusterClient) *Server {
	return &Server{readDB: readDB, writeDB: writeDB, rdb: rdb}
}

func main() {
	port := config.Getenv("PORT", "8092")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	portInt, _ := strconv.Atoi(port)
	log.Printf("[%s] starting on :%s", serviceName, port)

	readDB := db.MustConnectDB(config.Getenv("READ_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_gamification")), serviceName)
	writeDB := db.MustConnectDB(config.Getenv("WRITE_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_gamification")), serviceName)
	log.Printf("[%s] CRDB pools ready (read+write)", serviceName)

	// Redis Cluster:排行榜 ZSET 主存(esp:gamif:leaderboard:{period})
	rdb := redisx.MustNewCluster(config.Getenv("REDIS_CLUSTER", ""), serviceName)

	srv := NewServer(readDB, writeDB, rdb)

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(serviceName, portInt)

	// 排行喂数(双源,偏差①):
	//   ① Pulsar CQRS 投影器 startRankingsProjector —— 订阅 learning.event.report 近实时投影(stub 默认 no-op,
	//      需 -tags pulsar_eda + PULSAR_ENABLED=true 启用);
	//   ② 轮询 pollLoop —— 兜底完整源(坑㉙:content 事件经 outbox 直插 learning_events 不走 Pulsar,落库表是唯一完整源)。
	//   两者经 message_id SETNX(偏差⑤)幂等互斥,先到者处理、后到者跳过,既不漏也不重。
	go srv.pollLoop()
	go srv.startRankingsProjector()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(100, 200))                                           // 100 rps,桶容量200(突发余量)
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))                // 5次失败触发30s熔断
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", srv.readyz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api/gamification")
	api.GET("/profile/:user_id", srv.getProfile)            // 积分/连胜画像
	api.POST("/points", srv.addPoints)                      // 加分(同步直调,也由 Pulsar 异步触发)
	api.GET("/leaderboard", srv.leaderboard)                // 排行榜(Redis ZSET 主)
	api.GET("/achievements/:user_id", srv.listAchievements) // 用户成就
	api.POST("/achievements/grant", srv.grantAchievement)   // 授予成就
	// 排行榜(功能拓展 Phase3)
	api.GET("/rankings", srv.rankings)                       // 双榜:type=xp|progress scope=global|region|circle|category
	api.GET("/rankings/gap", srv.rankingGap)                 // 我的名次/差距/percentile
	api.GET("/rankings/regions", srv.regions)                // 可用地域清单(去重自 users.region_code)
	api.POST("/users/optout", srv.setOptOut)                 // 排行榜 opt-out 开关
	// 数字人形象偏好(per-user)
	api.GET("/me/digital-human", srv.getDigitalHuman)
	api.PUT("/me/digital-human", srv.setDigitalHuman)

	httpSrv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[%s] shutting down", serviceName)
	registry.DeRegister(serviceName, portInt, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func (s *Server) readyz(c *gin.Context) {
	if err := s.readDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "db_read": err.Error()})
		return
	}
	// Redis cluster 探测
	if err := s.rdb.Ping(c.Request.Context()).Err(); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "redis": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "UP", "db_read": true, "redis": true})
}

// 积分/连胜画像(读 user_streaks + achievements)
// 表:user_streaks(user_id,current_days,longest_days,last_active,updated_at)
// 表:achievements(id,code,name,description,icon_url,criteria_json)
func (s *Server) getProfile(c *gin.Context) {
	uid := c.Param("user_id")
	var currentDays, longestDays int64
	var lastActive sql.NullString
	_ = s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT current_days, longest_days, last_active FROM user_streaks WHERE user_id=$1", uid).
		Scan(&currentDays, &longestDays, &lastActive)

	c.JSON(200, gin.H{
		"user_id":        uid,
		"current_days":   currentDays,
		"longest_streak": longestDays,
		"last_active":    lastActive.String,
	})
}

// 加分:更新连胜(user_streaks upsert)+ Redis ZSET 同步(双写,Redis 主、CRDB 周期快照)
// scaffold:积分暂以连胜 current_days+1 体现(真实积分表待 esp_gamification 扩展 profiles)
func (s *Server) addPoints(c *gin.Context) {
	var body struct {
		UserID int64  `json:"user_id"`
		Points int64  `json:"points"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// DB:user_streaks upsert(current_days 累加,体现活跃);CRDB PG 方言 ON CONFLICT。
	_, err := s.writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO user_streaks (user_id, current_days, longest_days, last_active, updated_at) VALUES ($1, 1, 1, CURRENT_DATE, now()) ON CONFLICT (user_id) DO UPDATE SET current_days = user_streaks.current_days + 1, longest_days = GREATEST(user_streaks.longest_days, user_streaks.current_days + 1), last_active = CURRENT_DATE, updated_at = now()",
		body.UserID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// Redis ZSET:周榜
	zkey := "esp:gamif:leaderboard:weekly"
	if err := s.rdb.ZIncrBy(c.Request.Context(), zkey, float64(body.Points), uidStr(body.UserID)).Err(); err != nil {
		log.Printf("[%s] redis ZIncrBy warn: %v", serviceName, err)
	}
	c.JSON(202, gin.H{"accepted": true, "points_added": body.Points, "reason": body.Reason})
}

// 排行榜(Redis ZSET 主)
func (s *Server) leaderboard(c *gin.Context) {
	period := c.DefaultQuery("period", "weekly")
	topStr := c.DefaultQuery("top", "10")
	zkey := "esp:gamif:leaderboard:" + period
	members, err := s.rdb.ZRevRangeWithScores(c.Request.Context(), zkey, 0, parseTop(topStr)).Result()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(members))
	for i, m := range members {
		out = append(out, gin.H{"rank": i + 1, "user_id": m.Member, "score": m.Score})
	}
	c.JSON(200, out)
}

// 用户成就
func (s *Server) listAchievements(c *gin.Context) {
	uid := c.Param("user_id")
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		"SELECT achievement_id, unlocked_at FROM user_achievements WHERE user_id=$1 ORDER BY unlocked_at DESC", uid)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var achID int64
		var unlockedAt sql.NullString
		_ = rows.Scan(&achID, &unlockedAt)
		out = append(out, gin.H{"achievement_id": achID, "unlocked_at": unlockedAt.String})
	}
	c.JSON(200, out)
}

// 授予成就
func (s *Server) grantAchievement(c *gin.Context) {
	var body struct {
		UserID        int64 `json:"user_id"`
		AchievementID int64 `json:"achievement_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	_, err := s.writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO user_achievements (user_id, achievement_id, unlocked_at) VALUES ($1, $2, now()) ON CONFLICT DO NOTHING",
		body.UserID, body.AchievementID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"granted": true})
}

func mustConnectDB(dsn string) *sql.DB {
	// 已迁移至 esp/common-go/db.MustConnectDB(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return db.MustConnectDB(dsn, serviceName)
}

func getenv(k, dft string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, dft)
}

func uidStr(i int64) string {
	return strconv.FormatInt(i, 10)
}

func parseTop(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 100 {
		n = 10
	}
	return n - 1
}
