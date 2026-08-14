package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	host    = flag.String("host", "http://gateway-go:80", "gateway host")
	bff     = flag.String("bff", "http://bff-java:8090", "bff host")
	vus     = flag.Int("vus", 50, "virtual users")
	rampS   = flag.Int("ramp", 10, "ramp seconds")
	steadyS = flag.Int("steady", 30, "steady seconds")
)

type stats struct {
	scenarios200 int64
	scenariosErr int64
	loginErr     int64
	totalDur     int64 // ns
	count        int64
}

func register(bffHost, email string) string {
	body, _ := json.Marshal(map[string]any{"email": email, "password": "test123", "name": email, "role": "learner"})
	resp, err := http.Post(bffHost+"/api/auth/register", "application/json", bytes.NewReader(body))
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var m map[string]any
			json.NewDecoder(resp.Body).Decode(&m)
			if t, ok := m["token"].(string); ok {
				return t
			}
		}
	}
	// try login
	body2, _ := json.Marshal(map[string]any{"email": email, "password": "test123"})
	resp2, err := http.Post(bffHost+"/api/auth/login", "application/json", bytes.NewReader(body2))
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode == 200 {
			var m map[string]any
			json.NewDecoder(resp2.Body).Decode(&m)
			if t, ok := m["token"].(string); ok {
				return t
			}
		}
	}
	return ""
}

func vu(id int, wg *sync.WaitGroup, s *stats, stop <-chan struct{}) {
	defer wg.Done()
	email := fmt.Sprintf("k6_%d@loadtest.com", id)
	token := register(*bff, email)
	if token == "" {
		atomic.AddInt64(&s.loginErr, 1)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		select {
		case <-stop:
			return
		default:
		}
		req, _ := http.NewRequest("GET", *host+"/api/scenarios", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		start := time.Now()
		resp, err := client.Do(req)
		dur := time.Since(start)
		atomic.AddInt64(&s.totalDur, int64(dur))
		atomic.AddInt64(&s.count, 1)
		if err != nil {
			atomic.AddInt64(&s.scenariosErr, 1)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			atomic.AddInt64(&s.scenarios200, 1)
		} else {
			atomic.AddInt64(&s.scenariosErr, 1)
		}
		time.Sleep(100 * time.Millisecond) // pacing
	}
}

func main() {
	flag.Parse()
	s := &stats{}
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// ramp
	rampInterval := time.Duration(*rampS*1000/ *vus) * time.Millisecond
	if rampInterval <= 0 {
		rampInterval = 100 * time.Millisecond
	}
	for i := 1; i <= *vus; i++ {
		wg.Add(1)
		go vu(i, &wg, s, stop)
		time.Sleep(rampInterval)
	}
	// steady
	time.Sleep(time.Duration(*steadyS) * time.Second)
	close(stop)
	wg.Wait()

	count := atomic.LoadInt64(&s.count)
	durMs := float64(atomic.LoadInt64(&s.totalDur)) / 1e6 / float64(count+1)
	fmt.Printf("\n=== k6go baseline 结果 ===\n")
	fmt.Printf("VUS=%d ramp=%ds steady=%ds\n", *vus, *rampS, *steadyS)
	fmt.Printf("scenarios 请求总数: %d\n", count)
	fmt.Printf("scenarios 200: %d  错误: %d  login失败: %d\n",
		atomic.LoadInt64(&s.scenarios200), atomic.LoadInt64(&s.scenariosErr), atomic.LoadInt64(&s.loginErr))
	if count > 0 {
		fmt.Printf("平均耗时: %.2f ms\n", durMs)
		fmt.Printf("吞吐: %.1f rps\n", float64(count)/float64(*steadyS))
		fmt.Printf("成功率: %.2f%%\n", 100*float64(atomic.LoadInt64(&s.scenarios200))/float64(count))
	}
}
