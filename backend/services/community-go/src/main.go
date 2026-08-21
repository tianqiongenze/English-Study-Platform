// community-go -- 圈子社区(功能拓展:组队/建圈共同学习、分享经验)
// 圈子(circle)-> 成员(membership)-> 帖子(post)-> 评论(comment)-> 举报(report)。
// esp_community 库。身份:BFF 注入可信 X-User-Id / X-Role(EdgeAuthHandler 已剥离外网伪造)。
// 社区治理:帖子/评论写时关键词过滤;举报达阈值自动隐藏(status=hidden)。
// 邀请码:6 位大写字母数字,创建时生成,防重试;member_limit 上限 500。
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/common-go/registry"
)

const serviceName = "community-go"

const (
	reportHideThreshold = 3  // 举报达阈值自动隐藏
	inviteCodeLen       = 6
	maxMemberLimit      = 500
)

// Server 持有依赖,所有 handler 通过方法绑定 -> 可注入 mock DB 做单测。
type Server struct {
	readDB  *sql.DB
	writeDB *sql.DB
	jwt     *middleware.JWTValidator
}

func NewServer(w, r *sql.DB, jwt *middleware.JWTValidator) *Server {
	return &Server{readDB: r, writeDB: w, jwt: jwt}
}

// blockedWords:写时关键词过滤(基础版,可扩展)。命中则拒绝发布。
var blockedWords = strings.Fields(config.Getenv("BLOCKED_WORDS",
	"赌博 诈骗 广告 加V 加微信 代考 包过 作弊 嫖娼 毒品 政治敏感"))

func main() {
	middleware.SetRateLimitService(serviceName)
	port := config.Getenv("PORT", "8097")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	rDB := db.MustConnectDB(config.Getenv("READ_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_community")), serviceName)
	wDB := db.MustConnectDB(config.Getenv("WRITE_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_community")), serviceName)
	log.Printf("[%s] CRDB pools ready (read+write)", serviceName)
	// P1-30 零信任:服务自身校验 BFF 签发的 JWT(RS256 公钥),不轻信 X-User-Id 头。
	jwtValidator := middleware.MustInitJWTFromEnv(serviceName)
	communitySrv := NewServer(wDB, rDB, jwtValidator)

	nacosCli := registry.RegisterNacos(serviceName, 8097)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(100, 200))
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", communitySrv.readyz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api/community")
	api.POST("/circles", communitySrv.createCircle)
	api.GET("/circles", communitySrv.listCircles)
	api.GET("/circles/mine", communitySrv.myCircles)
	api.GET("/circles/:id", communitySrv.getCircle)
	api.GET("/circles/:id/members", communitySrv.listMembers)
	api.POST("/circles/:id/join", communitySrv.joinCircle)
	api.POST("/circles/:id/leave", communitySrv.leaveCircle)
	api.POST("/circles/:id/posts", communitySrv.createPost)
	api.GET("/circles/:id/posts", communitySrv.listPosts)
	api.POST("/posts/:id/comments", communitySrv.createComment)
	api.GET("/posts/:id/comments", communitySrv.listComments)
	api.POST("/reports", communitySrv.createReport)

	// 全语言统一深度诊断面板:debug 端点(actuator 等价),由 admin-go handleDebugProxy 代理 + RequireAdminDebug 守门。
	r.Any("/debug/*path", gin.WrapH(middleware.GinDebugGroup(middleware.DebugRoutes())))

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
	registry.DeRegister(serviceName, 8097, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func (s *Server) readyz(c *gin.Context) {
	if err := s.readDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "db_read": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "UP", "db_read": true})
}

// ---- 身份/鉴权:零信任 JWT 自校验(P1-30)----
// 此前纯信任 BFF 注入的 X-User-Id 头(集群内直连即可冒充);现改为服务自身用
// BFF 签发 JWT 的 RS256 公钥校验取 userID,不读 X-User-Id 头。

// userID 从请求 JWT(RS256 公钥验签)取 userID;token 缺失/非法返 0。
func (s *Server) userID(c *gin.Context) int64 {
	return middleware.UserIDFromRequest(s.jwt, c.Request)
}

func (s *Server) requireAuth(c *gin.Context) (int64, bool) {
	uid := s.userID(c)
	if uid <= 0 {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return 0, false
	}
	return uid, true
}

// ---- 圈子 ----

type circleOut struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	InviteCode  string `json:"invite_code"`
	OwnerID     int64  `json:"owner_id"`
	OwnerName   string `json:"owner_name"`
	MemberLimit int64  `json:"member_limit"`
	MemberCount int64  `json:"member_count"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	Joined      bool   `json:"joined"`
	MyRole      string `json:"my_role,omitempty"`
}

func (s *Server) createCircle(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	var b struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		MemberLimit int64  `json:"member_limit"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len([]rune(b.Name)) > 64 {
		c.JSON(400, gin.H{"error": "name required, <=64 chars"})
		return
	}
	if b.MemberLimit <= 0 || b.MemberLimit > maxMemberLimit {
		b.MemberLimit = 50
	}

	var circleID int64
	for attempt := 0; attempt < 5; attempt++ {
		code := genInviteCode()
		err := s.writeDB.QueryRowContext(c.Request.Context(),
			"INSERT INTO circles (name, description, invite_code, owner_id, member_limit) VALUES ($1,$2,$3,$4,$5) RETURNING id",
			b.Name, b.Description, code, uid, b.MemberLimit).Scan(&circleID)
		if err == nil {
			break
		}
		if attempt == 4 {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	_, _ = s.writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO circle_memberships (circle_id, user_id, role) VALUES ($1,$2,'owner') ON CONFLICT (circle_id, user_id) DO UPDATE SET role='owner'",
		circleID, uid)
	c.JSON(201, gin.H{"id": circleID, "invite_code": s.getInviteCode(c, circleID)})
}

func (s *Server) getInviteCode(c *gin.Context, circleID int64) string {
	var code string
	_ = s.writeDB.QueryRowContext(c.Request.Context(), "SELECT invite_code FROM circles WHERE id=$1", circleID).Scan(&code)
	return code
}

func (s *Server) listCircles(c *gin.Context) {
	uid := s.userID(c)
	q := c.DefaultQuery("q", "")
	status := c.DefaultQuery("status", "active")
	var rows *sql.Rows
	var err error
	if q != "" {
		rows, err = s.readDB.QueryContext(c.Request.Context(),
			"SELECT id, name, description, invite_code, owner_id, member_limit, status, created_at FROM circles WHERE status=$1 AND name ILIKE $2 ORDER BY id",
			status, "%"+q+"%")
	} else {
		rows, err = s.readDB.QueryContext(c.Request.Context(),
			"SELECT id, name, description, invite_code, owner_id, member_limit, status, created_at FROM circles WHERE status=$1 ORDER BY id",
			status)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, ownerID, limit int64
		var name, desc, code, st string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &desc, &code, &ownerID, &limit, &st, &createdAt); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": id, "name": name, "description": desc, "invite_code": code,
			"owner_id": ownerID, "owner_name": s.userDisplayName(c, ownerID),
			"member_limit": limit, "member_count": s.memberCount(c, id),
			"status": st, "created_at": createdAt.UTC().Format(time.RFC3339),
			"joined": s.isMember(c, id, uid), "my_role": s.memberRole(c, id, uid),
		})
	}
	c.JSON(200, out)
}

func (s *Server) myCircles(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		`SELECT ci.id, ci.name, ci.description, ci.invite_code, ci.owner_id, ci.member_limit, ci.status, ci.created_at, cm.role
		 FROM circle_memberships cm JOIN circles ci ON ci.id = cm.circle_id
		 WHERE cm.user_id=$1 AND ci.status='active' ORDER BY cm.joined_at DESC`, uid)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, ownerID, limit int64
		var name, desc, code, st, role string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &desc, &code, &ownerID, &limit, &st, &createdAt, &role); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": id, "name": name, "description": desc, "invite_code": code,
			"owner_id": ownerID, "owner_name": s.userDisplayName(c, ownerID),
			"member_limit": limit, "member_count": s.memberCount(c, id),
			"status": st, "created_at": createdAt.UTC().Format(time.RFC3339),
			"joined": true, "my_role": role,
		})
	}
	c.JSON(200, out)
}

func (s *Server) getCircle(c *gin.Context) {
	uid := s.userID(c)
	id := c.Param("id")
	cid, _ := strconv.ParseInt(id, 10, 64)
	var name, desc, code, st string
	var ownerID, limit int64
	var createdAt time.Time
	err := s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT name, description, invite_code, owner_id, member_limit, status, created_at FROM circles WHERE id=$1 AND status='active'", id).
		Scan(&name, &desc, &code, &ownerID, &limit, &st, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "circle not found"})
		return
	}
	c.JSON(200, gin.H{
		"id": id, "name": name, "description": desc, "invite_code": code,
		"owner_id": ownerID, "owner_name": s.userDisplayName(c, ownerID),
		"member_limit": limit, "member_count": s.memberCount(c, cid),
		"status": st, "created_at": createdAt.UTC().Format(time.RFC3339),
		"joined": s.isMember(c, cid, uid), "my_role": s.memberRole(c, cid, uid),
	})
}

func (s *Server) listMembers(c *gin.Context) {
	id := c.Param("id")
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		`SELECT m.user_id, m.role, m.joined_at, u.name FROM circle_memberships m
		 LEFT JOIN esp_auth.users u ON u.id = m.user_id
		 WHERE m.circle_id=$1 ORDER BY m.role DESC, m.joined_at ASC`, id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var userID int64
		var role string
		var joinedAt time.Time
		var name sql.NullString
		if err := rows.Scan(&userID, &role, &joinedAt, &name); err != nil {
			continue
		}
		out = append(out, gin.H{
			"user_id": userID, "name": name.String, "role": role,
			"joined_at": joinedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(200, out)
}

// joinCircle 原子化(消除 TOCTOU):
//   - 旧实现 isMember(member 从 follower 读)+ memberCount(follower 读)+ INSERT(写主库)三步非原子,
//     follower lag 下"查通过但写时状态已变"+ 并发超 member_limit。CLC/RWT 铁律修法。
//   - 新实现:单事务 BeginTx 主库,SELECT ... FOR UPDATE 锁 circles 行(序列化同圈子入圈操作),
//     事务内 COUNT 成员 + INSERT ... ON CONFLICT DO NOTHING;受影响行 0=已加入返幂等提示;
//     member_limit 满事务内判定拒绝。check+act 同事务同主库,无 follower lag 漂移。
func (s *Server) joinCircle(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	id := c.Param("id")
	cid, _ := strconv.ParseInt(id, 10, 64)
	var b struct {
		InviteCode string `json:"invite_code"`
	}
	_ = c.BindJSON(&b)

	ctx := c.Request.Context()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer func() { _ = tx.Rollback() }() // 失败/已 Commit 后回滚为 no-op

	var circleName, code string
	var ownerID, limit int64
	// FOR UPDATE 锁圈子行:并发入圈请求在此排队,避免同时通过容量检查后各自插入超限。
	err = tx.QueryRowContext(ctx,
		"SELECT name, invite_code, owner_id, member_limit FROM circles WHERE id=$1 AND status='active' FOR UPDATE",
		cid).Scan(&circleName, &code, &ownerID, &limit)
	if err != nil {
		c.JSON(404, gin.H{"error": "circle not found"})
		return
	}
	if strings.ToUpper(strings.TrimSpace(b.InviteCode)) != code {
		c.JSON(403, gin.H{"error": "invalid invite code"})
		return
	}
	// 事务内从主库 COUNT(非 follower),与 INSERT 同事务消除 check-then-act 漂移。
	var cnt int64
	if err := tx.QueryRowContext(ctx,
		"SELECT count(*) FROM circle_memberships WHERE circle_id=$1", cid).Scan(&cnt); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if cnt >= limit {
		c.JSON(403, gin.H{"error": "circle member limit reached"})
		return
	}
	// INSERT ... ON CONFLICT DO NOTHING:PK(circle_id,user_id) 提供原子幂等;受影响行判定是否首次加入。
	res, err := tx.ExecContext(ctx,
		"INSERT INTO circle_memberships (circle_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING",
		cid, uid)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// 已加入(幂等):不报错,返友好标识。事务提交保持不变。
		if err := tx.Commit(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"id": id, "already_member": true})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": id, "joined": true})
}

func (s *Server) leaveCircle(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	id := c.Param("id")
	cid, _ := strconv.ParseInt(id, 10, 64)
	if s.memberRole(c, cid, uid) == "owner" {
		c.JSON(403, gin.H{"error": "owner cannot leave; transfer ownership first"})
		return
	}
	_, _ = s.writeDB.ExecContext(c.Request.Context(),
		"DELETE FROM circle_memberships WHERE circle_id=$1 AND user_id=$2", id, uid)
	c.JSON(200, gin.H{"id": id, "left": true})
}

// ---- 帖子与评论 ----

func (s *Server) createPost(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	circleID := c.Param("id")
	cid, _ := strconv.ParseInt(circleID, 10, 64)
	if !s.isMember(c, cid, uid) {
		c.JSON(403, gin.H{"error": "join circle to post"})
		return
	}
	var b struct {
		Body string `json:"body"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	b.Body = strings.TrimSpace(b.Body)
	if b.Body == "" || len([]rune(b.Body)) > 2000 {
		c.JSON(400, gin.H{"error": "body required, <=2000 chars"})
		return
	}
	if word, hit := filterBlocked(b.Body); hit {
		c.JSON(403, gin.H{"error": "content blocked by keyword filter", "word": word})
		return
	}
	var postID int64
	err := s.writeDB.QueryRowContext(c.Request.Context(),
		"INSERT INTO circle_posts (circle_id, user_id, body) VALUES ($1,$2,$3) RETURNING id",
		circleID, uid, b.Body).Scan(&postID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": postID})
}

func (s *Server) listPosts(c *gin.Context) {
	circleID := c.Param("id")
	cursor := c.DefaultQuery("cursor", "")
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	var rows *sql.Rows
	var err error
	if cursor != "" {
		rows, err = s.readDB.QueryContext(c.Request.Context(),
			`SELECT p.id, p.user_id, u.name, p.body, p.status, p.report_count, p.created_at,
			        (SELECT count(*) FROM circle_post_comments cc WHERE cc.post_id=p.id AND cc.status='published')
			 FROM circle_posts p LEFT JOIN esp_auth.users u ON u.id = p.user_id
			 WHERE p.circle_id=$1 AND p.status='published' AND p.id < $2
			 ORDER BY p.id DESC LIMIT $3`, circleID, cursor, limit)
	} else {
		rows, err = s.readDB.QueryContext(c.Request.Context(),
			`SELECT p.id, p.user_id, u.name, p.body, p.status, p.report_count, p.created_at,
			        (SELECT count(*) FROM circle_post_comments cc WHERE cc.post_id=p.id AND cc.status='published')
			 FROM circle_posts p LEFT JOIN esp_auth.users u ON u.id = p.user_id
			 WHERE p.circle_id=$1 AND p.status='published'
			 ORDER BY p.id DESC LIMIT $2`, circleID, limit)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var pid, userID, reportCount, commentCount int64
		var body, st string
		var name sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&pid, &userID, &name, &body, &st, &reportCount, &createdAt, &commentCount); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": pid, "user_id": userID, "user_name": name.String, "body": body,
			"status": st, "report_count": reportCount, "comment_count": commentCount,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(200, out)
}

func (s *Server) createComment(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	postID := c.Param("id")
	var circleID int64
	err := s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT circle_id FROM circle_posts WHERE id=$1 AND status='published'", postID).Scan(&circleID)
	if err != nil {
		c.JSON(404, gin.H{"error": "post not found"})
		return
	}
	if !s.isMember(c, circleID, uid) {
		c.JSON(403, gin.H{"error": "join circle to comment"})
		return
	}
	var b struct {
		Body string `json:"body"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	b.Body = strings.TrimSpace(b.Body)
	if b.Body == "" || len([]rune(b.Body)) > 500 {
		c.JSON(400, gin.H{"error": "body required, <=500 chars"})
		return
	}
	if word, hit := filterBlocked(b.Body); hit {
		c.JSON(403, gin.H{"error": "content blocked by keyword filter", "word": word})
		return
	}
	var commentID int64
	err = s.writeDB.QueryRowContext(c.Request.Context(),
		"INSERT INTO circle_post_comments (post_id, user_id, body) VALUES ($1,$2,$3) RETURNING id",
		postID, uid, b.Body).Scan(&commentID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": commentID})
}

func (s *Server) listComments(c *gin.Context) {
	postID := c.Param("id")
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		`SELECT cc.id, cc.user_id, u.name, cc.body, cc.created_at FROM circle_post_comments cc
		 LEFT JOIN esp_auth.users u ON u.id = cc.user_id
		 WHERE cc.post_id=$1 AND cc.status='published' ORDER BY cc.id ASC`, postID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var cid, userID int64
		var body string
		var name sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&cid, &userID, &name, &body, &createdAt); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": cid, "user_id": userID, "user_name": name.String, "body": body,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(200, out)
}

// ---- 举报与自动隐藏 ----

func (s *Server) createReport(c *gin.Context) {
	uid, ok := s.requireAuth(c)
	if !ok {
		return
	}
	var b struct {
		TargetType string `json:"target_type"` // post | comment
		TargetID   int64  `json:"target_id"`
		Reason     string `json:"reason"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if b.TargetType != "post" && b.TargetType != "comment" {
		c.JSON(400, gin.H{"error": "target_type must be post|comment"})
		return
	}
	var okTarget bool
	if b.TargetType == "post" {
		_ = s.readDB.QueryRowContext(c.Request.Context(),
			"SELECT true FROM circle_posts WHERE id=$1", b.TargetID).Scan(&okTarget)
	} else {
		_ = s.readDB.QueryRowContext(c.Request.Context(),
			"SELECT true FROM circle_post_comments WHERE id=$1", b.TargetID).Scan(&okTarget)
	}
	if !okTarget {
		c.JSON(404, gin.H{"error": "target not found"})
		return
	}
	res, err := s.writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO content_reports (target_type, target_id, reporter_id, reason) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING",
		b.TargetType, b.TargetID, uid, b.Reason)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(200, gin.H{"idempotent": true, "message": "already reported"})
		return
	}
	var cnt int64
	_ = s.writeDB.QueryRowContext(c.Request.Context(),
		"SELECT count(*) FROM content_reports WHERE target_type=$1 AND target_id=$2 AND status='pending'",
		b.TargetType, b.TargetID).Scan(&cnt)
	if cnt >= reportHideThreshold {
		if b.TargetType == "post" {
			_, _ = s.writeDB.ExecContext(c.Request.Context(),
				"UPDATE circle_posts SET status='hidden', report_count=$1 WHERE id=$2", cnt, b.TargetID)
		} else {
			_, _ = s.writeDB.ExecContext(c.Request.Context(),
				"UPDATE circle_post_comments SET status='hidden', report_count=$1 WHERE id=$2", cnt, b.TargetID)
		}
	}
	c.JSON(201, gin.H{"report_count": cnt, "auto_hidden": cnt >= reportHideThreshold})
}

// ---- helpers ----

func (s *Server) memberCount(c *gin.Context, circleID int64) int64 {
	var n int64
	_ = s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT count(*) FROM circle_memberships WHERE circle_id=$1", circleID).Scan(&n)
	return n
}

func (s *Server) isMember(c *gin.Context, circleID int64, uid int64) bool {
	var one bool
	_ = s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT true FROM circle_memberships WHERE circle_id=$1 AND user_id=$2", circleID, uid).Scan(&one)
	return one
}

func (s *Server) memberRole(c *gin.Context, circleID int64, uid int64) string {
	var role string
	_ = s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT role FROM circle_memberships WHERE circle_id=$1 AND user_id=$2", circleID, uid).Scan(&role)
	return role
}

func (s *Server) userDisplayName(c *gin.Context, uid int64) string {
	var name string
	_ = s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT name FROM esp_auth.users WHERE id=$1", uid).Scan(&name)
	return name
}

// filterBlocked:关键词过滤,返回命中词(未命中返回 false)。
func filterBlocked(body string) (string, bool) {
	lower := strings.ToLower(body)
	for _, w := range blockedWords {
		if w != "" && strings.Contains(lower, strings.ToLower(w)) {
			return w, true
		}
	}
	return "", false
}

// genInviteCode:6 位大写字母数字(crypto/rand 安全随机)。
func genInviteCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去掉易混淆 O/I/0/1
	// P1-D18(2026-08-21,SCS):消除模偏差。原 int(idx)%len(chars) 因 256%31=8 致前 8 字符
	// 多一个被选概率。改拒绝法(rejection sampling):只接受落在 [0, maxBiasFree) 的随机字节,
	// maxBiasFree = 256 - 256%len(chars),保证每个字符等概率(无偏);超出则丢弃重取。
	maxBiasFree := 256 - 256%len(chars)
	b := make([]byte, inviteCodeLen)
	for i := 0; i < inviteCodeLen; i++ {
		var rb [1]byte
		for {
			if _, err := rand.Read(rb[:]); err != nil {
				// crypto/rand 失败(极罕见):降级时间戳,不阻塞建圈主流程
				return strconv.FormatInt(time.Now().UnixNano(), 10)[:inviteCodeLen]
			}
			if int(rb[0]) < maxBiasFree {
				b[i] = chars[int(rb[0])%len(chars)]
				break
			}
		}
	}
	return string(b)
}

// randID:无外部依赖的随机 hex(预留 message_id 幂等键)。
func randID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}
