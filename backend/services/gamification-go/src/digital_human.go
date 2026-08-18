// 数字人形象偏好(per-user digital human):GET/PUT esp_auth.users.digital_human。
// 写端 bff 主拥有 users,但 gamification-go 已实证可写 esp_auth.users(setOptOut,跨 schema);
// 此端点让前端把 persona 偏好跨设备持久化(前端默认按 userId 确定性派生,选择后存这)。
// 校验:persona_id 限 1..32,字符集 [a-z0-9-],后端不绑定具体预设清单(前端 persona 表为准)。

package main

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

var personaIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func requireUID(c *gin.Context) (int64, bool) {
	meStr := c.GetHeader("X-User-Id")
	if meStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return 0, false
	}
	uid, err := strconv.ParseInt(meStr, 10, 64)
	if err != nil || uid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad user"})
		return 0, false
	}
	return uid, true
}

func (s *Server) getDigitalHuman(c *gin.Context) {
	uid, ok := requireUID(c)
	if !ok {
		return
	}
	var persona sql.NullString
	err := s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT digital_human FROM esp_auth.users WHERE id=$1", uid).Scan(&persona)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"persona_id": persona.String})
}

func (s *Server) setDigitalHuman(c *gin.Context) {
	uid, ok := requireUID(c)
	if !ok {
		return
	}
	var body struct {
		PersonaID string `json:"persona_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !personaIDRe.MatchString(body.PersonaID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad persona_id"})
		return
	}
	if _, err := s.writeDB.ExecContext(c.Request.Context(),
		"UPDATE esp_auth.users SET digital_human=$1 WHERE id=$2", body.PersonaID, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"persona_id": body.PersonaID})
}
