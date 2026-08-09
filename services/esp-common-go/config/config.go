// Package config —— ESP Go 服务统一 env 读取(会话22-3 债1+2)。
// 消除 7 Go 服务里 getenv/env/envInt/getenvDefault/getenvDefaultInt 共 4 种命名变体。
package config

import "os"

// Getenv 读环境变量,空值返回默认值。对齐各服务原 getenv 行为(空串视为未设置)。
func Getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// GetenvInt 读环境变量为 int,空或非法返回默认值。对齐 gateway-go envInt 行为。
func GetenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n := 0
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return def
		}
		n = n*10 + int(v[i]-'0')
	}
	return n
}
