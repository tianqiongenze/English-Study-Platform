// gamification-go —— 学习目标/连胜/排行榜/成就/积分商城(S7, P1, 事件驱动)
// scaffold 级:健康检查 + 积分/连胜/排行榜/成就路由桩 + Pulsar 订阅 esp.business.*(桩)
// ADR: docs/design/adr-new-services.md / PRD §七 / schema esp_gamification
//
// 事件驱动:Pulsar 订阅 esp.business.* 主题,学习/练习/批改事件 → 积分/连胜/成就。
// scaffold 阶段 Pulsar consumer 用占位 goroutine(真实接入待 Pulsar Go client)。
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
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

const serviceName = "gamification-go"

var (
	readDB  *sql.DB
	writeDB *sql.DB
	rdb     *redis.ClusterClient
)

func main() {
	port := getenv("PORT", "8092")
	gin.SetMode(getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	readDB = mustConnectDB(getenv("READ_DB_URL", "root:@tcp(tidb-follower:4000)/esp_gamification?parseTime=true"))
	writeDB = mustConnectDB(getenv("WRITE_DB_URL", "root:@tcp(tidb-primary:4000)/esp_gamification?parseTime=true"))
	log.Printf("[%s] TiDB pools ready (read+write)", serviceName)

	// Redis Cluster:排行榜 ZSET 主存(esp:gamif:leaderboard:{period})
	redisAddr := getenv("REDIS_CLUSTER", "redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379")
	rdb = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    []string{redisAddr},
		PoolSize: 16,
	})
	log.Printf("[%s] Redis Cluster client ready", serviceName)

	// Pulsar 订阅 esp.business.*(scaffold 占位 goroutine)
	// 真实接入:用 apache/pulsar-client-go Subscribe topic=esp.business.* subscription=gamification
	// 收到事件 → applyEvent(event) → 积分/连胜/成就更新。当前仅打日志占位。
	go pulsarEventLoopStub()

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP", "service": serviceName}) })
	r.GET("/livez", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP"}) })
	r.GET("/readyz", readyz)

	api := r.Group("/api/gamification")
	api.GET("/profile/:user_id", getProfile)            // 积分/连胜画像
	api.POST("/points", addPoints)                      // 加分(同步直调,也由 Pulsar 异步触发)
	api.GET("/leaderboard", leaderboard)                // 排行榜(Redis ZSET 主)
	api.GET("/achievements/:user_id", listAchievements) // 用户成就
	api.POST("/achievements/grant", grantAchievement)   // 授予成就

	srv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[%s] shutting down", serviceName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func readyz(c *gin.Context) {
	if err := readDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "db_read": err.Error()})
		return
	}
	// Redis cluster 探测
	if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "redis": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "UP", "db_read": true, "redis": true})
}

// 积分/连胜画像(读 user_streaks + achievements)
// 表:user_streaks(user_id,current_days,longest_days,last_active,updated_at)
// 表:achievements(id,code,name,description,icon_url,criteria_json)
func getProfile(c *gin.Context) {
	uid := c.Param("user_id")
	var currentDays, longestDays int64
	var lastActive sql.NullString
	_ = readDB.QueryRowContext(c.Request.Context(),
		"SELECT current_days, longest_days, last_active FROM user_streaks WHERE user_id=?", uid).
		Scan(&currentDays, &longestDays, &lastActive)

	c.JSON(200, gin.H{
		"user_id":        uid,
		"current_days":   currentDays,
		"longest_streak": longestDays,
		"last_active":    lastActive.String,
	})
}

// 加分:更新连胜(user_streaks upsert)+ Redis ZSET 同步(双写,Redis 主、TiDB 周期快照)
// scaffold:积分暂以连胜 current_days+1 体现(真实积分表待 esp_gamification 扩展 profiles)
func addPoints(c *gin.Context) {
	var body struct {
		UserID int64  `json:"user_id"`
		Points int64  `json:"points"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// DB:user_streaks upsert(current_days 累加,体现活跃)
	_, err := writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO user_streaks (user_id, current_days, longest_days, last_active, updated_at) VALUES (?, 1, 1, CURDATE(), NOW()) ON DUPLICATE KEY UPDATE current_days=current_days+1, longest_days=GREATEST(longest_days, current_days+1), last_active=CURDATE(), updated_at=NOW()",
		body.UserID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// Redis ZSET:周榜
	zkey := "esp:gamif:leaderboard:weekly"
	if err := rdb.ZIncrBy(c.Request.Context(), zkey, float64(body.Points), uidStr(body.UserID)).Err(); err != nil {
		log.Printf("[%s] redis ZIncrBy warn: %v", serviceName, err)
	}
	c.JSON(202, gin.H{"accepted": true, "points_added": body.Points, "reason": body.Reason})
}

// 排行榜(Redis ZSET 主)
func leaderboard(c *gin.Context) {
	period := c.DefaultQuery("period", "weekly")
	topStr := c.DefaultQuery("top", "10")
	zkey := "esp:gamif:leaderboard:" + period
	members, err := rdb.ZRevRangeWithScores(c.Request.Context(), zkey, 0, parseTop(topStr)).Result()
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
func listAchievements(c *gin.Context) {
	uid := c.Param("user_id")
	rows, err := readDB.QueryContext(c.Request.Context(),
		"SELECT achievement_id, unlocked_at FROM user_achievements WHERE user_id=? ORDER BY unlocked_at DESC", uid)
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
func grantAchievement(c *gin.Context) {
	var body struct {
		UserID        int64 `json:"user_id"`
		AchievementID int64 `json:"achievement_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	_, err := writeDB.ExecContext(c.Request.Context(),
		"INSERT IGNORE INTO user_achievements (user_id, achievement_id, unlocked_at) VALUES (?,?,NOW())",
		body.UserID, body.AchievementID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"granted": true})
}

// Pulsar 事件循环占位:真实接入用 pulsar-client-go。
// 当前仅周期打日志,表示事件汇聚点存活。
func pulsarEventLoopStub() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		log.Printf("[%s] pulsar consumer stub alive (esp.business.* — 真实接入待 pulsar-client-go)", serviceName)
	}
}

func mustConnectDB(dsn string) *sql.DB {
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

func getenv(k, dft string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return dft
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
