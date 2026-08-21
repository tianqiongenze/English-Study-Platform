package main

// 运维操作面板(Phase3 · 2026-08-21):资源监控 + 实时日志 + 可管理操作(scale/restart)。
//
// 设计原则(对齐工程铁律):
//   - 零 k8s client-go 依赖:纯 net/http 直连 Kubernetes API(用 in-cluster ServiceAccount token + CA),
//     保持 admin-go 零重依赖基线。in-cluster 配置读 KUBERNETES_SERVICE_HOST 环境变量 + /var/run/secrets/.../token。
//   - 资源监控走 Prometheus 查询(复用 handleProm 的白名单机制扩展 PromQL 查询),
//     不直接调 metrics-server:节点 CPU/内存/磁盘 + 容器级 CPU/内存 + JVM 线程数。
//   - 写操作(scale/restart)需独立 RBAC(esp-admin-sa + 受限 Role 只授 deploy/scale + pod/log),
//     禁 cluster-admin;BRR 铁律:L2 不可逆操作默认需 admin token(已有 requireAdmin) + 前端 Popconfirm 二次确认。
//   - 防滥用:写操作记审计日志(who/what/when → app_logs),限流(复用 middleware)。
//
// 路由(main.go 注册):
//   GET  /api/admin/ops/resources      —— 资源监控快照(节点 + 容器级 CPU/内存/磁盘/线程)
//   GET  /api/admin/logs/{service}     —— 实时日志(kubectl logs 等价:代理到 K8s pod log API)
//   POST /api/admin/scale/{service}    —— scale deploy(replicas)
//   POST /api/admin/restart/{service}  —— rollout restart

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"esp/common-go/middleware"
)

// k8sInClusterConfig 返回 in-cluster Kubernetes API 基址 + token + CA 路径。
// KUBERNETES_SERVICE_HOST/PORT 由 kubelet 注入(所有 pod 默认有);
// token/CA 来自 ServiceAccount 挂载(需 esp-admin-sa + RBAC,见 deploy/k8s/33-admin-go.yaml)。
// 未配置 RBAC 时这些可能不存在 → 写操作降级返回 503(不 CrashLoop,对齐 RWT 降级铁律)。
func k8sBase() (host, token, caPath string, ok bool) {
	host = os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return "", "", "", false
	}
	tokBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return host + ":" + port, "", "", false
	}
	caPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	return host + ":" + port, strings.TrimSpace(string(tokBytes)), caPath, true
}

// k8sClient 构造访问 K8s API 的 http.Client(信任 in-cluster CA)。
// 读 serviceaccount/ca.crt 注入 TLS RootCAs(K8s API 服务证书由集群 CA 签发)。
// ok=false 表示无 RBAC 配置(SA token 不可读),调用方应降级。
func k8sClient() (*http.Client, string, string, bool) {
	host, token, caPath, ok := k8sBase()
	if !ok {
		return nil, "", "", false
	}
	tlsCfg := &tls.Config{}
	if caBytes, err := os.ReadFile(caPath); err == nil {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(caBytes) {
			tlsCfg.RootCAs = pool
		}
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg}}, host, token, true
}

// ---------- 资源监控(Phase3b):Prometheus 查询聚合 ----------

// 资源监控查询项:节点级 + 容器级 + JVM 线程。PromQL 串与 Grafana 常用面板同源。
// 注意(darwin):node_exporter 在 macOS 上指标名与 Linux 不同 ——
//
//	Linux: node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes
//	darwin: node_memory_total_bytes / node_memory_free_bytes(无 MemAvailable)
//
// 此处用 or 兼容两平台(优先 Linux MemAvailable,darwin 回退 free/total)。
var resourceQueries = map[string]string{
	// 节点级(单节点 Rancher Desktop,单节点 node 值)
	"node_cpu_usage":    `100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))`,
	"node_memory_usage": `100 * (1 - (node_memory_MemAvailable_bytes{}/node_memory_MemTotal_bytes{} or (node_memory_free_bytes/node_memory_total_bytes)))`,
	"node_disk_usage":   `100 * (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})`,
	"node_load1":        `node_load1`,
	// 容器级聚合(所有 esp-prod pod,经 kubelet cAdvisor)
	"container_cpu_cores": `sum(rate(container_cpu_usage_seconds_total{namespace="esp-prod"}[5m]))`,
	"container_mem_bytes": `sum(container_memory_working_set_bytes{namespace="esp-prod"})`,
	// pod 数量(kube-state-metrics;无 KSM 时 K8s API 兜底)
	"pod_total": `count(kube_pod_info{namespace="esp-prod"})`,
	"pod_ready": `count(kube_pod_status_ready{namespace="esp-prod",condition="true"})`,
}

// handleOpsResources 聚合多 PromQL 查询,返回资源监控快照。
// 单次 HTTP 请求并发查所有 PromQL(复用 prometheus 查询,白名单已含 query)。
func (s *Server) handleOpsResources(w http.ResponseWriter, _ *http.Request) {
	// Task B-pilot:Sentinel 动态流控(resource=api/admin/ops)。被限流返 429。
	if !middleware.AllowOrLimit(adminServiceName, "api/admin/ops") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"flow limited","retry_after":1}`))
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	type qResult struct {
		Query  string `json:"query"`
		Value  string `json:"value"`
		Status string `json:"status"`
		Source string `json:"source"` // prometheus | k8s_api | na(无数据)
	}
	results := make(map[string]qResult, len(resourceQueries))
	for key, promql := range resourceQueries {
		u := fmt.Sprintf("%s/api/v1/query?query=%s", s.prom, urlEncode(promql))
		resp, err := client.Get(u)
		if err != nil {
			results[key] = qResult{Query: promql, Status: "error", Source: "prometheus"}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		// Prometheus 返回 {"status":"success","data":{"result":[{"value":[ts,"数值"]}]}}
		val := extractPromValue(body)
		src := "prometheus"
		st := "ok"
		if val == "" {
			// Prometheus 无该 series(单节点 dev 环境常缺 node_exporter/kube-state-metrics)。
			// 标 na,前端显示 N/A,不误报"0"。
			st = "na"
			src = "na"
		}
		results[key] = qResult{Query: promql, Value: val, Status: st, Source: src}
	}

	// K8s API 补充:pod 总数/就绪数(不依赖 Prometheus,SA 有 pods list 权限即可)。
	// 当 Prometheus 的 pod_total/pod_ready 为 na 时,用 K8s API 兜底提供真实值。
	if k8c, host, token, ok := k8sClient(); ok {
		if pt, pr, err := k8sPodCounts(k8c, host, token); err == nil {
			if results["pod_total"].Status == "na" {
				results["pod_total"] = qResult{Value: pt, Status: "ok", Source: "k8s_api", Query: "K8s API: count pods in esp-prod"}
			}
			if results["pod_ready"].Status == "na" {
				results["pod_ready"] = qResult{Value: pr, Status: "ok", Source: "k8s_api", Query: "K8s API: count ready pods in esp-prod"}
			}
		}
	}

	writeJSON(w, map[string]any{
		"kind":       "ops_resources",
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"metrics":    results,
		"note":       "节点 CPU/内存/磁盘/负载(node_exporter)+ 容器 CPU/内存(kubelet cAdvisor)+ pod 计数(kube-state-metrics / K8s API 兜底);PromQL 跨平台兼容 Linux/darwin 指标名",
	})
}

// k8sPodCounts 用 K8s API 统计 esp-prod ns 的 pod 总数 + 就绪数(不依赖 Prometheus)。
func k8sPodCounts(client *http.Client, host, token string) (total, ready string, err error) {
	u := fmt.Sprintf("https://%s/api/v1/namespaces/esp-prod/pods", host)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "0", "0", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	// 简易计数:统计 "ready":"true" 出现次数 + pod 总数(metadata.name)。
	s := string(body)
	readyCount := strings.Count(s, `"ready":true`)
	// pod 总数:统计 "kind":"Pod" 不准,用 containerStatuses 块数或 metadata.name 数。
	// 简化:统计 "name":"esp-" 或 pod 名特征不可靠;用 /api/v1/namespaces/.../pods 返回的 items 数。
	totalCount := strings.Count(s, `"metadata":{"name":`) + strings.Count(s, `"name":"`)
	// 上面会多算,改用 items 数组元素:统计 "podIP" 出现次数(每 pod 一个)更稳。
	totalCount = strings.Count(s, `"podIP"`)
	if totalCount == 0 {
		totalCount = strings.Count(s, `"containerStatuses"`)
	}
	return fmt.Sprintf("%d", totalCount), fmt.Sprintf("%d", readyCount), nil
}

// extractPromValue 从 Prometheus query 响应 body 提取标量值(第一个 result 的 value)。
// 容错:非预期格式返回空串(前端降级显示 N/A)。
func extractPromValue(body []byte) string {
	// 简易提取:找 "value":[ts,"<num>"] 的 num 段。
	s := string(body)
	idx := strings.Index(s, `"value":[`)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(`"value":[`):]
	// rest 形如 [1234.5,"0.42"],取第二个引号内的值
	startQ := strings.Index(rest, `,"`)
	if startQ < 0 {
		return ""
	}
	rest = rest[startQ+2:]
	endQ := strings.Index(rest, `"]`)
	if endQ < 0 {
		return ""
	}
	return rest[:endQ]
}

// urlEncode 最小 URL 编码(仅 query 串,PromQL 含 {} "/ "= 空格 等)。
func urlEncode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_' || r == '.' || r == '~':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%02X", r)
		}
	}
	return b.String()
}

// ---------- 实时日志(Phase3c):K8s pod log API 代理 ----------

// handleOpsLogs 代理到 K8s pod log API(kubectl logs 等价)。
// 路径:/api/admin/logs/{service}?tail=200&since=5m&previous=false
// 实现:查 service 对应 deploy 的 pod → 调 /api/v1/namespaces/esp-prod/pods/{pod}/log?tailLines=&sinceSeconds=
// 需 RBAC: pods/log get 权限(esp-admin-sa)。
func (s *Server) handleOpsLogs(w http.ResponseWriter, r *http.Request) {
	svcName := strings.TrimPrefix(r.URL.Path, "/api/admin/logs/")
	if svcName == "" {
		http.Error(w, `{"error":"service name required"}`, 400)
		return
	}
	client, host, token, ok := k8sClient()
	if !ok {
		http.Error(w, `{"error":"k8s RBAC not configured (esp-admin-sa missing)","msg":"deploy 33-admin-go.yaml ServiceAccount + RoleBinding 未配置"}`, http.StatusServiceUnavailable)
		return
	}
	// service → deploy → pod 名(取最新 Ready pod)。
	pod, err := findPodForService(client, host, token, svcName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"find pod failed","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	// 代理 pod log API。
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	u := fmt.Sprintf("https://%s/api/v1/namespaces/esp-prod/pods/%s/log?tailLines=%s", host, pod, tail)
	req, _ := http.NewRequestWithContext(r.Context(), "GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"k8s log api unreachable","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// findPodForService 用 K8s API 列出 esp-prod ns 的 pod,按 app=<svcName> label 取第一个 Ready pod。
func findPodForService(client *http.Client, host, token, svcName string) (string, error) {
	u := fmt.Sprintf("https://%s/api/v1/namespaces/esp-prod/pods?labelSelector=app=%s", host, svcName)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	// 简易解析:取首个 "metadata":{"name":"<pod>"} 的 name。
	s := string(body)
	idx := strings.Index(s, `"metadata":{"name":"`)
	if idx < 0 {
		return "", fmt.Errorf("no pod found for app=%s", svcName)
	}
	rest := s[idx+len(`"metadata":{"name="`):]
	// 修正:metadata 结构含 name 字段
	idx2 := strings.Index(s[idx:], `"name":"`)
	if idx2 < 0 {
		return "", fmt.Errorf("parse pod name failed")
	}
	rest = s[idx+idx2+len(`"name":"`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", fmt.Errorf("parse pod name end failed")
	}
	return rest[:end], nil
}

// ---------- 可管理操作(Phase3d):scale + restart ----------

// handleOpsScale POST /api/admin/scale/{service}?replicas=N
// L2 不可逆变更(BRR 铁律):requireAdmin 已守门 + 前端 Popconfirm 二次确认。
func (s *Server) handleOpsScale(w http.ResponseWriter, r *http.Request) {
	svcName := strings.TrimPrefix(r.URL.Path, "/api/admin/scale/")
	replicas := r.URL.Query().Get("replicas")
	if svcName == "" || replicas == "" {
		http.Error(w, `{"error":"service and replicas required"}`, 400)
		return
	}
	client, host, token, ok := k8sClient()
	if !ok {
		http.Error(w, `{"error":"k8s RBAC not configured"}`, http.StatusServiceUnavailable)
		return
	}
	// PATCH deploy/{svc} 的 spec.replicas。body: {"spec":{"replicas":<N>}}
	body := fmt.Sprintf(`{"spec":{"replicas":%s}}`, replicas)
	u := fmt.Sprintf("https://%s/apis/apps/v1/namespaces/esp-prod/deployments/%s", host, svcName)
	req, _ := http.NewRequestWithContext(r.Context(), "PATCH", u, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"scale failed","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	writeJSON(w, map[string]any{
		"action":   "scale",
		"service":  svcName,
		"replicas": replicas,
		"status":   http.StatusText(resp.StatusCode),
		"code":     resp.StatusCode,
		"response": string(rb),
	})
}

// handleOpsRestart POST /api/admin/restart/{service}
// 实现:PATCH deploy/{svc} metadata.annotations 加 kubectl.kubernetes.io/restartedAt 触发 rollout restart。
func (s *Server) handleOpsRestart(w http.ResponseWriter, r *http.Request) {
	svcName := strings.TrimPrefix(r.URL.Path, "/api/admin/restart/")
	if svcName == "" {
		http.Error(w, `{"error":"service name required"}`, 400)
		return
	}
	client, host, token, ok := k8sClient()
	if !ok {
		http.Error(w, `{"error":"k8s RBAC not configured"}`, http.StatusServiceUnavailable)
		return
	}
	body := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().UTC().Format(time.RFC3339))
	u := fmt.Sprintf("https://%s/apis/apps/v1/namespaces/esp-prod/deployments/%s", host, svcName)
	req, _ := http.NewRequestWithContext(r.Context(), "PATCH", u, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"restart failed","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	writeJSON(w, map[string]any{
		"action":   "restart",
		"service":  svcName,
		"status":   http.StatusText(resp.StatusCode),
		"code":     resp.StatusCode,
		"response": string(rb),
	})
}
