// outbox_relay.go -- CRDB 侧 outbox 重投器(content-go 自持,镜像 learning-java OutboxRelay)。
// 背景(坑㉙):content-go 的作答事件走 esp_learning.learning_event_outbox(CRDB),但唯一既有
//   OutboxRelay 在 learning-java,读的是 TiDB esp.learning_event_outbox(其 writeJdbc->tidb)。
//   两侧同表名不同库 -> content 事件永远到不了 CRDB esp_learning.learning_events ->
//   gamification-go 排行榜喂数(poll CRDB learning_events)恒空。本 relay 补上 CRDB 侧消费:
//   every 2s 扫 pending/failed 行 -> 批量 INSERT esp_learning.learning_events
//   (ON CONFLICT message_id DO NOTHING 幂等)-> done;失败指数退避(retry_count),>=8 置 dead。
package main

import (
	"context"
	"log"
	"time"
)

const (
	relayInterval  = 2 * time.Second
	relayBatch     = 200
	relayMaxRetry  = 8
	relayInsertSQL = `INSERT INTO esp_learning.learning_events (user_id, event_type, payload, message_id)
		VALUES ($1,$2,$3::jsonb,$4) ON CONFLICT (message_id) DO NOTHING`
	relayPollSQL = `SELECT id, message_id, user_id, event_type, payload, retry_count
		FROM esp_learning.learning_event_outbox
		WHERE status IN ('pending','failed') AND retry_count < 8 AND next_retry_at <= $1
		ORDER BY id LIMIT $2`
	relayDoneSQL   = `UPDATE esp_learning.learning_event_outbox SET status='done' WHERE id=$1`
	relayDeadSQL   = `UPDATE esp_learning.learning_event_outbox SET status='failed', retry_count=$1 WHERE id=$2`
	relayBackoffSQL = `UPDATE esp_learning.learning_event_outbox
		SET retry_count=$1, next_retry_at=now() + ($2 || ' seconds')::interval WHERE id=$3`
)

type relayRow struct {
	id         int64
	messageID  string
	userID     int64
	eventType  string
	payload    string
	retryCount int
}

func (s *Server) startOutboxRelay() {
	go func() {
		log.Printf("[%s] CRDB outbox relay started (every %s)", serviceName, relayInterval)
		ticker := time.NewTicker(relayInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.drainOutbox()
		}
	}()
}

func (s *Server) drainOutbox() {
	ctx := context.Background()
	rows, err := s.writeDB.QueryContext(ctx, relayPollSQL, time.Now().UTC(), relayBatch)
	if err != nil {
		log.Printf("[%s] outbox relay poll err: %v", serviceName, err)
		return
	}
	var batch []relayRow
	for rows.Next() {
		var r relayRow
		if err := rows.Scan(&r.id, &r.messageID, &r.userID, &r.eventType, &r.payload, &r.retryCount); err != nil {
			continue
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("[%s] outbox relay scan err: %v", serviceName, err)
		return
	}
	if len(batch) == 0 {
		return
	}

	// 批量写 learning_events(幂等,message_id 冲突即视为已投)
	var failIDs []int64
	for _, r := range batch {
		if _, err := s.writeDB.ExecContext(ctx, relayInsertSQL, r.userID, r.eventType, r.payload, r.messageID); err != nil {
			failIDs = append(failIDs, r.id)
			if len(failIDs) == 1 {
				log.Printf("[%s] outbox relay insert err (row %d): %v", serviceName, r.id, err)
			}
		}
	}
	if len(failIDs) == 0 {
		for _, r := range batch {
			if _, err := s.writeDB.ExecContext(ctx, relayDoneSQL, r.id); err != nil {
				log.Printf("[%s] outbox relay mark-done err id=%d: %v", serviceName, r.id, err)
			}
		}
		return
	}
	// 部分失败:失败行退避/死信,成功行 done
	failed := map[int64]bool{}
	for _, id := range failIDs {
		failed[id] = true
	}
	for _, r := range batch {
		if !failed[r.id] {
			if _, err := s.writeDB.ExecContext(ctx, relayDoneSQL, r.id); err != nil {
				log.Printf("[%s] outbox relay mark-done err id=%d: %v", serviceName, r.id, err)
			}
			continue
		}
		nextRetry := r.retryCount + 1
		if nextRetry >= relayMaxRetry {
			if _, err := s.writeDB.ExecContext(ctx, relayDeadSQL, nextRetry, r.id); err != nil {
				log.Printf("[%s] outbox relay mark-dead err id=%d: %v", serviceName, r.id, err)
			}
			continue
		}
		secs := relayBackoffSec(nextRetry)
		if _, err := s.writeDB.ExecContext(ctx, relayBackoffSQL, nextRetry, secs, r.id); err != nil {
			log.Printf("[%s] outbox relay backoff err id=%d: %v", serviceName, r.id, err)
		}
	}
}

func relayBackoffSec(retry int) int {
	switch retry {
	case 1:
		return 1
	case 2:
		return 5
	case 3:
		return 30
	default:
		return 300
	}
}
