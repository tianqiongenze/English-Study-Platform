//go:build pulsar_eda

// rankings_projector.go —— Pulsar CQRS 投影器(偏差①):订阅 learning.event.report,
// 近实时把学情事件投影到排行榜 ZSET(替代/并行轮询)。
//
// 设计依据:docs/design/architecture/phase3-rankings-design.md §8 偏差①。
// 隔离模式:对齐 conversation-go/src/pulsar(producer.go + producer_stub.go):
//   - build tag `pulsar_eda` 启用真实 pulsar-client-go(本文件);
//   - 默认编译 stub(rankings_projector_stub.go)no-op,保持 `go build ./src` 不拉 Pulsar 依赖。
//
// 订阅:topic = learning.event.report(EventTopics.LEARNING_EVENT_REPORT,与 learning-java 同源);
//   Shared 订阅名 "esp-gamif-rankings-projector"(多副本共享消费,每消息仅一副本处理)。
// 载荷:LearningEventReport JSON {userId, eventType, payload, messageId}(与 learning-java 反序列化结构一致)。
//
// 双源约束(坑㉙):content-go 的 content 事件经 outbox→OutboxRelay 直插 learning_events,
//   **不走 Pulsar**。因此 Pulsar 投影器只能覆盖走 Pulsar 的事件;轮询 pollLoop 仍作为
//   "完整源兜底"始终在跑(落库表是唯一完整源)。两者经 message_id SETNX(偏差⑤)幂等互斥,
//   同一事件无论被谁先处理,另一方都跳过 —— 既不漏(Pulsar 漏的轮询补)也不重(SETNX 去重)。
//
// 降级:PULSAR_ENABLED!=true 时 startRankingsProjector 直接 return(本地/CI/单节点默认不连 Pulsar);
//   client/consumer 初始化失败只 warn 不 panic(排行喂数退回轮询,服务可用性不受影响)。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

const (
	rankingsProjectorTopic        = "learning.event.report"
	rankingsProjectorSubscription = "esp-gamif-rankings-projector"
)

var (
	pulsarURLProj    = getenvProj("PULSAR_URL", "pulsar-broker:6650")
	pulsarEnableProj = getenvProj("PULSAR_ENABLED", "false") == "true" || getenvProj("PULSAR_ENABLED", "") == "1"
)

func getenvProj(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// LearningEventReport:与 learning-java LearningService.LearningEventReport 同构。
// payload 为原始 JSON(透传,不二次解析为强类型,保留事件载荷灵活性)。
type LearningEventReport struct {
	UserID    int64           `json:"userId"`
	EventType string          `json:"eventType"`
	Payload   json.RawMessage `json:"payload"`
	MessageID string          `json:"messageId"`
}

var (
	projOnce    sync.Once
	projClient  pulsar.Client
	projConsumer pulsar.Consumer
	projInitErr error
)

func getRankingsConsumer() (pulsar.Consumer, error) {
	if !pulsarEnableProj {
		return nil, nil
	}
	projOnce.Do(func() {
		c, err := pulsar.NewClient(pulsar.ClientOptions{
			URL:              "pulsar://" + pulsarURLProj,
			OperationTimeout: 3 * time.Second,
		})
		if err != nil {
			projInitErr = err
			return
		}
		con, err := c.Subscribe(pulsar.ConsumerOptions{
			Topic:                       rankingsProjectorTopic,
			SubscriptionName:            rankingsProjectorSubscription,
			Type:                        pulsar.Shared,
			SubscriptionInitialPosition: pulsar.SubscriptionPositionLatest,
		})
		if err != nil {
			projInitErr = err
			c.Close()
			return
		}
		projClient = c
		projConsumer = con
		log.Printf("[%s] rankings projector consumer ready topic=%s sub=%s url=%s",
			serviceName, rankingsProjectorTopic, rankingsProjectorSubscription, pulsarURLProj)
	})
	if projInitErr != nil {
		return nil, projInitErr
	}
	return projConsumer, nil
}

// startRankingsProjector 启动 Pulsar CQRS 投影循环(偏差①)。PULSAR_ENABLED 关闭时 no-op。
// pollLoop 始终并行在跑作完整源兜底(坑㉙:content 事件不走 Pulsar)。
func (s *Server) startRankingsProjector() {
	if !pulsarEnableProj {
		log.Printf("[%s] rankings projector disabled (PULSAR_ENABLED not true) — pollLoop is sole feed source", serviceName)
		return
	}
	con, err := getRankingsConsumer()
	if err != nil || con == nil {
		log.Printf("[%s] rankings projector consumer unavailable, fall back to poll-only: %v", serviceName, err)
		return
	}
	ctx := context.Background()
	log.Printf("[%s] rankings projector loop started", serviceName)
	for {
		msg, err := con.Receive(ctx)
		if err != nil {
			// 网络瞬断等:退避重试,不退出(退出则 Pulsar 源彻底失效,只剩轮询)
			log.Printf("[%s] rankings projector receive err: %v (retry in 2s)", serviceName, err)
			time.Sleep(2 * time.Second)
			continue
		}
		s.processProjectorMessage(ctx, msg)
		con.Ack(msg)
	}
}

// processProjectorMessage 把单条 Pulsar 事件投影到排行榜(复用 fanout 路径)。
// 幂等(偏差⑤):message_id SETNX,与轮询侧互斥 —— 先到者处理,后到者跳过。
func (s *Server) processProjectorMessage(ctx context.Context, msg pulsar.Message) {
	var ev LearningEventReport
	if err := json.Unmarshal(msg.Payload(), &ev); err != nil {
		log.Printf("[%s] rankings projector unmarshal err: %v (skip)", serviceName, err)
		return
	}
	if ev.UserID <= 0 {
		return
	}
	// 偏差⑤:message_id 去重。Pulsar 重投递同 messageId → SETNX 失败 → 跳过。
	// 无 messageId 时退化为 Pulsar MessageID 字符串(仍唯一)。
	msgID := ev.MessageID
	if msgID == "" {
		msgID = fmt.Sprintf("pulsar:%s", msg.ID().Serialize())
	}
	if !s.processedMark(ctx, msgID) {
		return // 已处理(轮询侧或重投递)
	}
	// 复用 fanout:需 userInfo(region/optOut)+ circles。Pulsar 投影器侧即时查(不依赖 pollLoop 的批量缓存)。
	le := learningEvent{
		ID:        0, // Pulsar 源无 DB id;message_id 去重不依赖此
		UserID:    ev.UserID,
		EventType: ev.EventType,
		Payload:   ev.Payload,
	}
	if err := s.loadUsers(ctx, []int64{ev.UserID}); err != nil {
		log.Printf("[%s] projector loadUsers err uid=%d: %v", serviceName, ev.UserID, err)
		return
	}
	userCacheMu.RLock()
	info, ok := userCache[ev.UserID]
	userCacheMu.RUnlock()
	if !ok {
		info = userInfo{} // 用户查不到(已删等):默认 region 空、optOut false,仍入 global 榜
	}
	circles, _ := s.memberCircleMap(ctx, []int64{ev.UserID})
	s.fanout(ctx, le, info, circles[ev.UserID])
}
