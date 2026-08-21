package main

// Pulsar MQ 消息查看(Phase4 · 2026-08-21)。
// 代理 Pulsar Admin REST 8080(broker headless svc pulsar-broker-0.pulsar-broker:8080,无认证已核实)。
// 范式仿 ops.go:212 handleOpsLogs 的 http.NewRequestWithContext 代理 + middleware.go:50 白名单防 SSRF。
//
// 路由(main.go 注册):
//   GET /api/admin/pulsar/topics?namespace=esp/business       —— topic 列表 + 积压/订阅 stats
//   GET /api/admin/pulsar/messages?topic=&subscription=&limit=10 —— peek 消息(只读不 ack)
//
// 铁律:
//   - BRR:全只读(GET topics/stats/messages peek),无 produce/ack/delete 路由。
//   - SCS:白名单 tenant=esp + ns∈{esp/business,esp/cache}(硬编码常量防越权);topic 正则防注入。
//   - RWT:下游 Pulsar 不可达返空 list + note,不 500 不阻塞。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// pulsarAllowedNS 白名单 namespace(防 SSRF/越权,仿 middleware.go:50 白名单范式)。
var pulsarAllowedNS = map[string]bool{
	"esp/business": true,
	"esp/cache":    true,
}

// topicNameRe topic 短名白名单(防注入)。
var topicNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// PulsarTopic Pulsar topic 元数据 + stats。
type PulsarTopic struct {
	Topic         string               `json:"topic"` // 短名 session.ended
	FQN           string               `json:"fqn"`   // persistent://esp/business/session.ended
	Subscriptions []PulsarSubscription `json:"subscriptions,omitempty"`
	Backlog       int                  `json:"backlog"`
	Producers     int                  `json:"producers"`
	MsgRateIn     string               `json:"msg_rate_in"`
	MsgRateOut    string               `json:"msg_rate_out"`
	Status        string               `json:"status"` // ok|down
}

// PulsarSubscription 订阅 + 积压。
type PulsarSubscription struct {
	Name             string `json:"name"`
	MsgBacklog       int    `json:"msg_backlog"`
	MsgRateOut       string `json:"msg_rate_out"`
	MsgRateRedeliver string `json:"msg_rate_redeliver"`
	Type             string `json:"type"` // Shared|Exclusive|Failover
}

// PulsarTopicsResponse topic 列表响应。
type PulsarTopicsResponse struct {
	Kind      string        `json:"kind"` // "pulsar_topics"
	CheckedAt string        `json:"checked_at"`
	Namespace string        `json:"namespace"`
	Topics    []PulsarTopic `json:"topics"`
	Note      string        `json:"note"`
}

// PulsarMessage peek 出的单条消息。
type PulsarMessage struct {
	MessageID   string          `json:"message_id"`
	PublishTime string          `json:"publish_time"`
	Producer    string          `json:"producer"`
	Payload     json.RawMessage `json:"payload"` // 尝试 JSON 美化,否则原文
	PayloadRaw  string          `json:"payload_raw"`
}

// PulsarMessagesResponse peek 消息响应。
type PulsarMessagesResponse struct {
	Kind         string          `json:"kind"` // "pulsar_messages"
	Topic        string          `json:"topic"`
	Subscription string          `json:"subscription"`
	Messages     []PulsarMessage `json:"messages"`
	Note         string          `json:"note"`
}

// handlePulsarTopics GET /api/admin/pulsar/topics?namespace=esp/business
// 列出 ns 下所有 topic + 每个 topic 的 stats(积压/订阅)。
func (s *Server) handlePulsarTopics(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "esp/business"
	}
	if !pulsarAllowedNS[ns] {
		http.Error(w, `{"error":"namespace not allowed, must be esp/business or esp/cache"}`, http.StatusBadRequest)
		return
	}
	tenant := "esp"
	// Fold1(2026-08-21):修 URL 双 esp bug —— ns 白名单是 FQN 形如 "esp/business",
	// 但 URL 模板 /admin/v2/namespaces/{tenant}/{ns}/topics 已带 tenant=esp,
	// 直接传 ns 会产出 /esp/esp/business/ → 404。这里取短名(business)。
	shortNS := strings.TrimPrefix(ns, "esp/")
	client := &http.Client{Timeout: 8 * time.Second}
	base := s.pulsarAdmin
	if base == "" {
		http.Error(w, `{"error":"PULSAR_ADMIN_URL not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// topic 列表:GET /admin/v2/namespaces/{tenant}/{ns}/topics
	listURL := fmt.Sprintf("%s/admin/v2/namespaces/%s/%s/topics", base, tenant, shortNS)
	req, _ := http.NewRequestWithContext(r.Context(), "GET", listURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, PulsarTopicsResponse{
			Kind: "pulsar_topics", CheckedAt: time.Now().UTC().Format(time.RFC3339),
			Namespace: ns, Topics: []PulsarTopic{}, Note: "Pulsar 不可达: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	lb, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	// 返回 ["persistent://esp/business/session.ended", ...] 或 {"errorCode":...}
	var fqnList []string
	if err := json.Unmarshal(lb, &fqnList); err != nil {
		writeJSON(w, PulsarTopicsResponse{
			Kind: "pulsar_topics", CheckedAt: time.Now().UTC().Format(time.RFC3339),
			Namespace: ns, Topics: []PulsarTopic{}, Note: "Pulsar topic 列表解析失败: " + err.Error(),
		})
		return
	}

	// 并发查每个 topic 的 stats(积压/订阅)
	topics := make([]PulsarTopic, len(fqnList))
	var wg sync.WaitGroup
	for i, fqn := range fqnList {
		wg.Add(1)
		go func(idx int, fqn string) {
			defer wg.Done()
			pt := PulsarTopic{FQN: fqn, Status: "ok"}
			// FQN persistent://esp/business/session.ended → short name = session.ended
			parts := strings.Split(fqn, "/")
			pt.Topic = parts[len(parts)-1]
			statsURL := fmt.Sprintf("%s/admin/v2/persistent/%s/%s/%s/stats", base, tenant, shortNS, pt.Topic)
			sreq, _ := http.NewRequestWithContext(r.Context(), "GET", statsURL, nil)
			sresp, serr := client.Do(sreq)
			if serr != nil {
				pt.Status = "down"
				topics[idx] = pt
				return
			}
			defer sresp.Body.Close()
			sb, _ := io.ReadAll(io.LimitReader(sresp.Body, 128<<10))
			var stats struct {
				Subscriptions map[string]struct {
					MsgBacklog       json.Number `json:"msgBacklog"`
					MsgRateOut       json.Number `json:"msgRateOut"`
					MsgRateRedeliver json.Number `json:"msgRateRedeliver"`
					Type             string      `json:"subscriptionType"`
				} `json:"subscriptions"`
				Backlog    json.Number `json:"backlogSize"`
				Producers  []struct{}  `json:"producers"`
				MsgRateIn  json.Number `json:"msgRateIn"`
				MsgRateOut json.Number `json:"msgRateOut"`
			}
			if jerr := json.Unmarshal(sb, &stats); jerr == nil {
				pt.MsgRateIn = string(stats.MsgRateIn)
				pt.MsgRateOut = string(stats.MsgRateOut)
				pt.Producers = len(stats.Producers)
				for name, sub := range stats.Subscriptions {
					pt.Subscriptions = append(pt.Subscriptions, PulsarSubscription{
						Name:             name,
						MsgBacklog:       parseIntSafe(string(sub.MsgBacklog)),
						MsgRateOut:       string(sub.MsgRateOut),
						MsgRateRedeliver: string(sub.MsgRateRedeliver),
						Type:             sub.Type,
					})
				}
			}
			topics[idx] = pt
		}(i, fqn)
	}
	wg.Wait()

	writeJSON(w, PulsarTopicsResponse{
		Kind:      "pulsar_topics",
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Namespace: ns,
		Topics:    topics,
		Note:      "Pulsar Admin REST 8080(无认证);topic 列表 + stats(积压/订阅);只读,不 ack 不重发(BRR 铁律)",
	})
}

// handlePulsarMessages GET /api/admin/pulsar/messages?topic=&subscription=&limit=10
// peek 消息(只读,不 ack 不 commit cursor)。Pulsar 3.x GET /subscription/{sub}/messages?messageLimit=N。
func (s *Server) handlePulsarMessages(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	sub := r.URL.Query().Get("subscription")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "10"
	}
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "esp/business"
	}
	if !pulsarAllowedNS[ns] {
		http.Error(w, `{"error":"namespace not allowed"}`, http.StatusBadRequest)
		return
	}
	if !topicNameRe.MatchString(topic) || sub == "" {
		http.Error(w, `{"error":"invalid topic or subscription"}`, http.StatusBadRequest)
		return
	}
	tenant := "esp"
	shortNS := strings.TrimPrefix(ns, "esp/")
	base := s.pulsarAdmin
	if base == "" {
		http.Error(w, `{"error":"PULSAR_ADMIN_URL not configured"}`, http.StatusServiceUnavailable)
		return
	}
	// peek URL:GET /admin/v2/persistent/{tenant}/{ns}/{topic}/subscription/{sub}/messages?messageLimit=N
	peekURL := fmt.Sprintf("%s/admin/v2/persistent/%s/%s/%s/subscription/%s/messages?messageLimit=%s",
		base, tenant, shortNS, url.PathEscape(topic), url.PathEscape(sub), url.QueryEscape(limit))
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", peekURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, PulsarMessagesResponse{
			Kind: "pulsar_messages", Topic: topic, Subscription: sub,
			Messages: []PulsarMessage{}, Note: "Pulsar peek 不可达: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	// Pulsar 返回单条 JSON 或数组 [{messageId, payloads, ...}]。容错解析。
	var msgs []PulsarMessage
	// 尝试数组
	if err := json.Unmarshal(rb, &msgs); err != nil {
		// 尝试单条
		var single struct {
			MessageID   string          `json:"messageId"`
			PublishTime string          `json:"publishTime"`
			Producer    string          `json:"producer"`
			Payload     json.RawMessage `json:"payload"`
		}
		if err2 := json.Unmarshal(rb, &single); err2 == nil && single.MessageID != "" {
			msgs = []PulsarMessage{{
				MessageID:   single.MessageID,
				PublishTime: single.PublishTime,
				Producer:    single.Producer,
				Payload:     single.Payload,
				PayloadRaw:  string(single.Payload),
			}}
		}
	}
	writeJSON(w, PulsarMessagesResponse{
		Kind: "pulsar_messages", Topic: topic, Subscription: sub,
		Messages: msgs,
		Note:     "peek 只读(不 ack 不 commit cursor,BRR 铁律);Pulsar 3.x /subscription/{sub}/messages?messageLimit=N",
	})
}

// parseIntSafe 容错解析整数,失败返 0。
func parseIntSafe(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}
