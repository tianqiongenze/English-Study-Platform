// Package ws 定义口语会话的 WebSocket 帧编解码与处理。
package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 写超时:所有 conn 写操作(普通帧 + 控制帧)统一 deadline,防对端不读导致写阻塞。
const writeTimeout = 10 * time.Second

// 帧类型
const (
	TypeAudioChunk = "audio_chunk"
	TypeTextInput  = "text_input"
	TypeAIReply    = "ai_reply"
	TypeCorrection = "correction"
	TypeHint       = "hint"
	TypeEnd        = "end"
	TypeError      = "error"
)

// Frame 是 WS 消息帧。
type Frame struct {
	Type       string   `json:"type"`
	Seq        int      `json:"seq,omitempty"`
	Data       string   `json:"data,omitempty"`
	Text       string   `json:"text,omitempty"`
	Rephrasing string   `json:"rephrasing,omitempty"`
	Hints      []string `json:"hints,omitempty"`
	Options    []string `json:"options,omitempty"`
	Original   string   `json:"original,omitempty"`
	Corrected  string   `json:"corrected,omitempty"`
	Phoneme    string   `json:"phoneme,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Message    string   `json:"message,omitempty"`
	// 数字人情绪驱动(#121):AI 回复帧附带情绪标签 + 口型序列,
	// 前端数字人按 visemes 时序驱动口型,按 emotion 切换表情基色。
	Emotion string   `json:"emotion,omitempty"`
	Visemes []Viseme `json:"visemes,omitempty"`
}

// Viseme 是一段口型:音素对应嘴型 + 起止毫秒(相对音频起点)。
type Viseme struct {
	ID  string `json:"id"`            // Preston Blair 口型编号(AA/E/O/MBP/FV 等)
	Start int    `json:"start"`         // 起始 ms
	End   int    `json:"end"`           // 结束 ms
}

// Encode 将 Frame 序列化为 JSON 字节。
func (f Frame) Encode() ([]byte, error) {
	return json.Marshal(f)
}

// DecodeFrame 从字节解析 Frame。
func DecodeFrame(b []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	if f.Type == "" {
		return f, fmt.Errorf("frame missing type")
	}
	return f, nil
}

// SafeConn 包装 gorilla WebSocket 连接,提供写互斥(G-04 修复)。
//
// gorilla/websocket 明确要求"同一连接最多一个并发写者"。gateway 中同一连接可能同时被
// 两条路径写入:
//   - 点对点响应:网关读循环内的 writeConn(handleUserText);
//   - Hub 广播:dispatch goroutine 的 sendLocal(跨端/跨 Pod 同步)。
//
// 故所有写操作(普通帧 + 控制帧 ping)必须经本包装的 writeMu 串行化。
type SafeConn struct {
	*websocket.Conn
	writeMu sync.Mutex
}

// NewSafeConn 包装一个已 upgrade 的 gorilla 连接。
func NewSafeConn(c *websocket.Conn) *SafeConn { return &SafeConn{Conn: c} }

// WriteFrame 序列化一帧并带 writeTimeout 写(线程安全)。
func (c *SafeConn) WriteFrame(f Frame) error {
	b, err := f.Encode()
	if err != nil {
		return err
	}
	return c.WriteBytes(b)
}

// WriteBytes 写已编码的帧字节(线程安全,带写超时)。Hub 广播一次编码多处复用,走此路径避免重复编码。
func (c *SafeConn) WriteBytes(b []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.Conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return c.Conn.WriteMessage(websocket.TextMessage, b)
}

// WriteControl 发送控制帧(心跳 ping),与普通写共用同一把互斥锁(gorilla 控制帧也不能并发)。
func (c *SafeConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.Conn.WriteControl(messageType, data, deadline)
}
