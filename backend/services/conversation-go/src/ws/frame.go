// Package ws 定义口语会话的 WebSocket 帧编解码与处理。
package ws

import (
	"encoding/json"
	"fmt"
)

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
