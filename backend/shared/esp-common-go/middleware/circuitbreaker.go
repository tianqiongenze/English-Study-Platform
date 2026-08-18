package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus 指标:熔断器状态 + 触发计数。
var (
	circuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state: 0=closed, 1=open, 2=half-open",
		},
		[]string{"service"},
	)
	circuitBreakerTrips = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_trips_total",
			Help: "Total number of circuit breaker trips (state -> open)",
		},
		[]string{"service"},
	)
)

func init() {
	prometheus.MustRegister(circuitBreakerState, circuitBreakerTrips)
}

// CircuitBreaker 熔断器(三态:Closed/Open/HalfOpen),并发安全。
type CircuitBreaker struct {
	failureThreshold int           // 触发熔断的失败次数
	recoveryTimeout  time.Duration // 熔断恢复超时(half-open 等待时间)
	failureCount     int
	lastFailureTime  time.Time
	state            string // closed/open/half-open
	mu               sync.Mutex
	serviceName      string // 用于 Prometheus 指标标签
}

// NewCircuitBreaker 创建熔断器,failures=触发阈值,timeout=恢复超时。
func NewCircuitBreaker(failures int, timeout time.Duration, serviceName string) *CircuitBreaker {
	cb := &CircuitBreaker{
		failureThreshold: failures,
		recoveryTimeout:  timeout,
		state:            "closed",
		serviceName:      serviceName,
	}
	// 初始状态:closed = 0
	if serviceName != "" {
		circuitBreakerState.WithLabelValues(serviceName).Set(0)
	}
	return cb
}

// stateValue 将字符串状态映射为 Prometheus gauge 值: closed=0, open=1, half-open=2。
func stateValue(s string) float64 {
	switch s {
	case "open":
		return 1
	case "half-open":
		return 2
	default:
		return 0
	}
}

// Allow 判断是否允许当前请求通过。
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case "open":
		if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = "half-open"
			if cb.serviceName != "" {
				circuitBreakerState.WithLabelValues(cb.serviceName).Set(stateValue("half-open"))
			}
			return true
		}
		return false
	case "half-open":
		return true // 允许一个请求试探
	default: // closed
		return true
	}
}

// RecordSuccess 记录成功,重置失败计数。
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = "closed"
	if cb.serviceName != "" {
		circuitBreakerState.WithLabelValues(cb.serviceName).Set(0)
	}
}

// RecordFailure 记录失败,达阈值触发熔断。
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	if cb.failureCount >= cb.failureThreshold {
		cb.state = "open"
		if cb.serviceName != "" {
			circuitBreakerState.WithLabelValues(cb.serviceName).Set(1)
			circuitBreakerTrips.WithLabelValues(cb.serviceName).Inc()
		}
	}
}

// GinCircuitBreaker 返回 Gin 中间件。
// serviceName 用于 Prometheus 指标标签(空串则不采集指标,向后兼容)。
func GinCircuitBreaker(failures int, timeout time.Duration, serviceName string) gin.HandlerFunc {
	cb := NewCircuitBreaker(failures, timeout, serviceName)
	return func(c *gin.Context) {
		if !cb.Allow() {
			c.JSON(503, gin.H{"error": "circuit breaker open"})
			c.Abort()
			return
		}
		c.Next()
		if c.Writer.Status() >= 500 {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	}
}

// HTTPCircuitBreaker 返回标准 http.Handler 中间件(供 net/http 服务如 gateway-go 使用)。
func HTTPCircuitBreaker(next http.Handler, failures int, timeout time.Duration, serviceName string) http.Handler {
	cb := NewCircuitBreaker(failures, timeout, serviceName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cb.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":"circuit breaker open"}`))
			return
		}
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if rw.status >= 500 {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	})
}

// statusRecorder 包装 ResponseWriter 以捕获响应状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rw *statusRecorder) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
