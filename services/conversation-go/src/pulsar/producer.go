//go:build pulsar_eda

// Package pulsar 封装 Pulsar producer(EDA):gateway-go 发 session.ended 事件到 learning-java。
//
// 设计:
//   - producer 延迟初始化(首次 publish 时创建),避免服务启动即连 Pulsar。
//   - sendAsync 失败降级:只 warn 不 panic(session End 正常完成,不被 EDA 阻断)。
//   - PULSAR_ENABLED=false 时 no-op(默认,本地/CI 环境)。
//   - PULSAR_URL 默认 pulsar-broker:6650(K8s ExternalName)。
package pulsar

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

var (
	pulsarURL    = getenv("PULSAR_URL", "pulsar-broker:6650")
	pulsarEnable = getenv("PULSAR_ENABLED", "false") == "true" || getenv("PULSAR_ENABLED", "") == "1"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	once     sync.Once
	client   pulsar.Client
	producer pulsar.Producer
	initErr  error
)

func getProducer() (pulsar.Producer, error) {
	if !pulsarEnable {
		return nil, nil
	}
	once.Do(func() {
		c, err := pulsar.NewClient(pulsar.ClientOptions{
			URL:              "pulsar://" + pulsarURL,
			OperationTimeout: 3 * time.Second,
		})
		if err != nil {
			initErr = err
			return
		}
		p, err := c.CreateProducer(pulsar.ProducerOptions{
			Topic:        "session.ended",
			Name:         "gateway-go",
			SendTimeout:  2 * time.Second,
		})
		if err != nil {
			initErr = err
			_ = c.Close()
			return
		}
		client = c
		producer = p
		slog.Info("pulsar producer ready", "topic", "session.ended", "url", pulsarURL)
	})
	if initErr != nil {
		return nil, initErr
	}
	return producer, nil
}

// PublishSessionEnded 异步发送 session.ended 事件;任何失败只 warn 不返回错误。
//
// payload 字段:session_id/user_id/scenario_id/turns。
func PublishSessionEnded(payload map[string]any) {
	if !pulsarEnable {
		return
	}
	p, err := getProducer()
	if err != nil || p == nil {
		slog.Warn("pulsar producer unavailable, skip session.ended (non-fatal)",
			"session_id", payload["session_id"], "err", err)
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("pulsar marshal session.ended failed (non-fatal)", "err", err)
		return
	}
	// sendAsync:失败在 callback 里 warn,不阻塞 End 主流程
	p.SendAsync(context.Background(), body, func(_ pulsar.MessageID, _ *pulsar.ProducerMessage, e error) {
		if e != nil {
			slog.Warn("pulsar send session.ended failed (non-fatal)",
				"session_id", payload["session_id"], "err", e)
		}
	})
}
