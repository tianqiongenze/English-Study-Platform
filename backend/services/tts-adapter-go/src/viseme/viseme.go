// Package viseme 提供 TTS 情绪感知与口型时间轴生成。
// 从 conversation-go/src/tts/mock.go 原样移植,行为完全一致。
//
// SenseEmotion: 关键词→四态契约(happy/neutral/encouraging/sad)
// BuildVisemes: 字符→Preston Blair 口型时间轴(60ms/viseme 均匀近似)
package viseme

import (
	"strings"
	"unicode"
)

// Viseme 一段口型:音素对应嘴型 + 起止毫秒(与 ws.Viseme 完全对齐)。
type Viseme struct {
	ID    string `json:"id"`            // Preston Blair 口型编号(AA/E/O/MBP/FV 等)
	Start int    `json:"start"`         // 起始 ms
	End   int    `json:"end"`           // 结束 ms
}

// ---- 情绪词表(前端 Avatar.tsx 四态契约) ----

var (
	negativeWords  = []string{"sad", "tired", "frustrated", "angry", "anxious", "nervous", "worried", "afraid", "stressed", "difficult", "hard", "fail", "bad"}
	positiveWords  = []string{"happy", "excited", "great", "good", "love", "confident", "proud", "easy", "fun", "wonderful", "amazing"}
	encourageWords = []string{"great job", "well done", "excellent", "nice work", "awesome", "perfect", "keep going", "good progress"}
)

// SenseEmotion 关键词情绪感知→四态契约(happy/neutral/encouraging/sad)。
// 规则顺序:鼓励词(条件正)→消极词(sad)→积极词(happy)→默认中性。
func SenseEmotion(text string) string {
	low := strings.ToLower(text)
	for _, w := range encourageWords {
		if strings.Contains(low, w) {
			return "encouraging"
		}
	}
	for _, w := range negativeWords {
		if strings.Contains(low, w) {
			return "sad"
		}
	}
	for _, w := range positiveWords {
		if strings.Contains(low, w) {
			return "happy"
		}
	}
	return "neutral"
}

// ---- 口型时间轴(字符→Preston Blair 口型) ----

func visemeForChar(r rune) string {
	switch {
	case unicode.ToLower(r) == 'o' || unicode.ToLower(r) == 'u':
		return "O"
	case unicode.ToLower(r) == 'e' || unicode.ToLower(r) == 'i':
		return "E"
	case unicode.ToLower(r) == 'a':
		return "AA"
	case r == 'm' || r == 'b' || r == 'p':
		return "MBP"
	case r == 'f' || r == 'v':
		return "FV"
	case unicode.IsLetter(r):
		return "etc"
	default:
		return ""
	}
}

// BuildVisemes 文本→口型时间轴。语速按 60ms/viseme 均匀近似(Mock 无真实时长)。
// 连续相同口型合并,序列长度上限 400(超长文本截断,防 WS 帧膨胀)。
func BuildVisemes(text string) []Viseme {
	var out []Viseme
	t := 0
	var prev string
	for _, r := range strings.ToLower(text) {
		v := visemeForChar(r)
		if v == "" {
			t += 30 // 非字母(空格/标点)短停顿
			continue
		}
		if v == prev && len(out) > 0 {
			out[len(out)-1].End = t + 60
			t += 60
			continue
		}
		out = append(out, Viseme{ID: v, Start: t, End: t + 60})
		prev = v
		t += 60
		if len(out) >= 400 {
			break
		}
	}
	return out
}
