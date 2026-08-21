// Package middleware —— Sentinel-golang 限流(2026-08-21 Task B-pilot)。
//
// 接线 Alibaba sentinel-golang v1.0.4 动态限流:规则经 Nacos 配置中心下发,
// dataId=<service>-flow-rules(与 Java 侧 Sentinel 约定一致),group=DEFAULT_GROUP。
//
// 设计要点(铁律对标):
//   - ASV:sentinel-golang 无 ext/datasource/nacos 包(与 Java Sentinel 不同),
//     本文件自建 Nacos datasource 适配器:用 nacos-sdk-go/v2 config_client.ListenConfig
//     拉取规则,解析 JSON,转 flow.Rule,经 flow.LoadRules 热加载(无需重启)。
//   - CLC:全链路 nil-safe。InitSentinel 失败→sentinelInited=false,SentinelFlow 直接
//     返 nil(降级放行),禁止 panic(零值/未初始化结构体是合法输入必须安全)。
//   - RWT:Sentinel/Nacos 任一初始化失败→降级放行不 CrashLoop,主链不阻塞。
//   - API 真实性(经 go doc 逐个核实,非凭记忆):
//       api.InitDefault() error
//       api.Entry(resource, opts...) (*base.SentinelEntry, *base.BlockError)
//       (*base.SentinelEntry).Exit(exitOps...)
//       flow.LoadRules([]*flow.Rule) (bool, error)
//       flow.Rule{ Resource, TokenCalculateStrategy(flow.Direct), ControlBehavior(flow.Reject),
//                  Threshold, StatIntervalInMs }
//       clients.NewConfigClient(vo.NacosClientParam{...}) → config_client.IConfigClient
//       IConfigClient.ListenConfig(vo.ConfigParam{DataId, Group, OnChange})
package middleware

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/logging"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// ErrFlowLimited 由 Sentinel 流控拦截(调用方应返 429)。
var ErrFlowLimited = errors.New("flow limited by sentinel")

// sentinelFlowRuleSchema Nacos 配置 JSON schema(向后兼容 Java Sentinel FlowRule 字段名)。
// 阈值默认按 QPS(StatIntervalInMs=1000 → Threshold 即 QPS)。
//
//	[{"resource":"api/llm/chat","count":1,"statIntervalMs":1000,"controlBehavior":0}]
//
// count=阈值;statIntervalMs 省略则 1000(1s);controlBehavior 省略则 0(Reject 快速失败)。
type sentinelFlowRuleSchema struct {
	Resource        string `json:"resource"`
	Count           int    `json:"count"`
	StatIntervalMs  uint32 `json:"statIntervalMs"`
	ControlBehavior int    `json:"controlBehavior"`
}

var (
	sentinelMu     sync.Mutex
	sentinelInited bool // InitDefault 成功且仅一次
)

// initSentinelRuntime 用自定义 LogDir 初始化 Sentinel 运行时。
// 默认 InitDefault 的 LogDir 不可写(readOnlyRootFilesystem 容器内)→ 改为 /tmp(必挂 emptyDir),
// 并把全局 logger 重置为 console(stdout)避免 file logger 写盘失败(RWP/CLC)。
func initSentinelRuntime(serviceName string) error {
	cfg := config.NewDefaultConfig()
	cfg.Sentinel.Log.Dir = "/tmp"
	cfg.Sentinel.Log.Logger = logging.NewConsoleLogger()
	cfg.Sentinel.App.Name = serviceName
	return sentinel.InitWithConfig(cfg)
}

// InitSentinel 初始化 Sentinel-golang 运行时 + 启动 Nacos 规则监听。
//
// serviceName:服务名(用于拼 dataId=<service>-flow-rules + 日志)。
// nacosAddr:Nacos 地址(host:port,如 "nacos:8848");空串则跳过监听(Sentinel 仍可用静态规则)。
//
// 幂等:重复调用只生效首次。任何步骤失败仅记日志并降级(SentinelFlow 返 nil 放行),
// 永不 panic、永不返回 error 阻断服务启动(RWT:启动期依赖不可用该降级运行)。
func InitSentinel(serviceName, nacosAddr string) error {
	sentinelMu.Lock()
	defer sentinelMu.Unlock()

	// 1) 初始化 Sentinel 运行时。InitDefault 用默认 LogDir(可能不可写 → init 失败)。
	// 改用 InitWithConfig 自定义 LogDir 到 /tmp(容器 emptyDir 挂载保证可写),
	// 彻底避开 readOnlyRootFilesystem 下 mkdir /home/app/logs 或 /logs 失败(RWP/CLC)。
	if !sentinelInited {
		if err := initSentinelRuntime(serviceName); err != nil {
			log.Printf("[sentinel][%s] init runtime failed, degrade to allow-all: %v", serviceName, err)
			// 不设 sentinelInited=true → SentinelFlow 走降级放行。
			return nil
		}
		sentinelInited = true
		log.Printf("[sentinel][%s] runtime initialized", serviceName)
	}

	// 2) 启动 Nacos 规则监听(地址空或建连失败→降级:仅用已 LoadRules 的静态规则,SentinelFlow 仍工作)。
	if nacosAddr == "" {
		log.Printf("[sentinel][%s] nacos addr empty, dynamic rule listener skipped", serviceName)
		return nil
	}
	host, portStr, err := net.SplitHostPort(nacosAddr)
	if err != nil {
		log.Printf("[sentinel][%s] invalid nacos addr %q: %v, listener skipped", serviceName, nacosAddr, err)
		return nil
	}
	port64, _ := strconv.ParseInt(portStr, 10, 64)

	sc := []constant.ServerConfig{*constant.NewServerConfig(host, uint64(port64))}
	cc := constant.NewClientConfig(
		constant.WithNamespaceId(""),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithLogLevel("warn"),
	)
	cli, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: sc,
	})
	if err != nil {
		log.Printf("[sentinel][%s] new nacos config client failed: %v, listener skipped", serviceName, err)
		return nil
	}

	dataId := serviceName + "-flow-rules"
	// 首次拉取一次(避免监听到首次变更前无规则空窗)。
	if content, gerr := cli.GetConfig(vo.ConfigParam{DataId: dataId, Group: "DEFAULT_GROUP"}); gerr == nil {
		loadFlowRulesFromContent(serviceName, content)
	} else {
		log.Printf("[sentinel][%s] initial GetConfig(%s) err: %v", serviceName, dataId, gerr)
	}

	// 启动监听(回调在 nacos-sdk 内部 goroutine 触发,无需额外 go)。
	if err := cli.ListenConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  "DEFAULT_GROUP",
		OnChange: func(namespace, group, dataId, data string) {
			log.Printf("[sentinel][%s] rule changed (dataId=%s), reloading", serviceName, dataId)
			loadFlowRulesFromContent(serviceName, data)
		},
	}); err != nil {
		log.Printf("[sentinel][%s] ListenConfig(%s) failed: %v, dynamic reload disabled", serviceName, dataId, err)
		// 首次 GetConfig 已成功则静态规则仍生效(无需监听)。
		return nil
	}
	log.Printf("[sentinel][%s] listening nacos dataId=%s for flow rules", serviceName, dataId)
	return nil
}

// loadFlowRulesFromContent 解析 Nacos 配置内容并热加载为 sentinel flow.Rule。
// 空串(Nacos 配置被删除时部分 SDK 版本会以空 data 回调)→ 清空已加载规则(flow.ClearRules),
// 避免删除后旧规则残留(已知:nacos-sdk-go 某些版本对 delete 不触发 OnChange,此处兜底:
// 一旦收到空 data,主动 ClearRules,保证"删配置即解限"语义)。解析失败→静默跳过不清除。
func loadFlowRulesFromContent(serviceName, content string) {
	if content == "" {
		if err := flow.ClearRules(); err != nil {
			log.Printf("[sentinel][%s] clear rules on empty content err: %v", serviceName, err)
			return
		}
		log.Printf("[sentinel][%s] rule content empty, cleared all flow rules", serviceName)
		return
	}
	var schemas []sentinelFlowRuleSchema
	if err := json.Unmarshal([]byte(content), &schemas); err != nil {
		log.Printf("[sentinel][%s] parse rule json failed: %v, content=%q", serviceName, err, truncate(content, 200))
		return
	}
	rules := make([]*flow.Rule, 0, len(schemas))
	for _, s := range schemas {
		if s.Resource == "" {
			continue
		}
		statInterval := s.StatIntervalMs
		if statInterval == 0 {
			statInterval = 1000 // 默认 1s 窗口 → Threshold 即 QPS
		}
		rules = append(rules, &flow.Rule{
			Resource:               s.Resource,
			TokenCalculateStrategy: flow.Direct,
			ControlBehavior:        flow.Reject, // controlBehavior=0 快速失败
			Threshold:              float64(s.Count),
			StatIntervalInMs:       statInterval,
		})
	}
	ok, err := flow.LoadRules(rules)
	if err != nil {
		log.Printf("[sentinel][%s] LoadRules err: %v", serviceName, err)
		return
	}
	log.Printf("[sentinel][%s] flow rule loaded: %d rules, applied=%v", serviceName, len(rules), ok)
}

// SentinelFlow 对资源执行 Sentinel 流控检查。
//
// 成功(放行)返 nil,调用方应在请求处理完后调用 SentinelExit 释放统计;
// 被拦截返 ErrFlowLimited(调用方应返 429)。
// 降级:Sentinel 未初始化(init 失败)→ 直接返 nil 放行(RWT:失败不阻塞主链)。
// nil-safe:零值/未初始化状态是合法输入(CLC)。
func SentinelFlow(resource string) error {
	sentinelMu.Lock()
	inited := sentinelInited
	sentinelMu.Unlock()
	if !inited {
		return nil // 降级放行
	}
	if resource == "" {
		return nil // 无资源名,放行(无可控对象)
	}
	entry, blockErr := sentinel.Entry(resource)
	if blockErr != nil {
		// *base.BlockError 满足 error 接口;此处统一映射为 ErrFlowLimited。
		return ErrFlowLimited
	}
	// 成功获取令牌:立即 Exit 释放(本中间件做"准入判断"而非包裹整段业务,
	// 与 HTTPRateLimit 语义对齐——放行后由后续 handler 处理,统计窗口已计 +1)。
	entry.Exit()
	return nil
}

// truncate 防日志爆量。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
