// live-tutor-go —— 真人外教 1v1 预约 + RTC 信令(N10,P1,scaffold 级)
// 单节点 RD:scaffold + 信令回声占位,RTC 真压留多节点。
// 完整 ADR/PRD/schema 见 docs/design/adr-new-services.md / prd-new-services.md / adr-schema-per-service.md
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const serviceName = "live-tutor-go"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func main() {
	port := getenv("PORT", "8094")
	log.Printf("[%s] starting on :%s", serviceName, port)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	mux := http.NewServeMux()
	mux.HandleFunc("/actuator/health", health)
	mux.HandleFunc("/actuator/health/readiness", readiness)
	mux.HandleFunc("/actuator/health/liveness", liveness)
	mux.HandleFunc("/api/tutor/book", book)        // 预约(seata 事务桩)
	mux.HandleFunc("/api/tutor/availability", availability) // 查可用时段(桩)
	mux.HandleFunc("/ws/signaling/", signaling)    // WebRTC 信令回声占位

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[%s] server error: %v", serviceName, err)
	}
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"status": "UP", "service": serviceName})
}
func readiness(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]any{"status": "UP"}) }
func liveness(w http.ResponseWriter, _ *http.Request)  { writeJSON(w, map[string]any{"status": "UP"}) }

// book —— 预约(桩:实际走 seata 事务 扣课时包 + 锁时段)
func book(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"booking_id": "bk-scaffold",
		"status":     "scaffold",
		"message":    "live-tutor book: seata tx stub, see PRD §3",
	})
}

func availability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"slots": []any{}, "status": "scaffold"})
}

// signaling —— WebRTC 信令回声占位(单节点验证握手;多节点接 SFU/MCU)
func signaling(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[%s] ws upgrade err: %v", serviceName, err)
		return
	}
	defer c.Close()
	for {
		mt, msg, err := c.ReadMessage()
		if err != nil {
			break
		}
		// 回声:单节点占位,验证信令通道连通
		if err := c.WriteMessage(mt, msg); err != nil {
			break
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
