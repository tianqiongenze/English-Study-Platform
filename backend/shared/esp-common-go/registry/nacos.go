// Package registry —— ESP Go 服务 Nacos 注册/注销(2026-08-17 迁移共享基础设施)。
// NACOS_ADDR 环境变量控制地址(默认 "nacos:8848"→ExternalName→ekb-m2 共享 nacos)。
// NACOS_IP 覆盖注册 IP(默认取 hostname 对应 Pod IP,适配 K8s headless/ClusterIP)。
// 未设置 NACOS_ADDR 或注册失败时静默跳过(不阻塞服务启动)。
package registry

import (
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// RegisterNacos 注册服务到 Nacos 并返回 naming_client(供 DeRegister 使用)。
// 返回 nil 表示未注册(NACOS_ADDR 为空或注册失败),调用方无需额外处理。
func RegisterNacos(serviceName string, port int) naming_client.INamingClient {
	addr := os.Getenv("NACOS_ADDR")
	if addr == "" {
		log.Printf("[%s] NACOS_ADDR not set, skipping nacos registration", serviceName)
		return nil
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("[%s] invalid NACOS_ADDR %q: %v, skipping nacos registration", serviceName, addr, err)
		return nil
	}
	port64, _ := strconv.ParseInt(portStr, 10, 64)

	sc := []constant.ServerConfig{
		*constant.NewServerConfig(host, uint64(port64)),
	}

	cc := constant.NewClientConfig(
		constant.WithNamespaceId(""),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithLogLevel("warn"),
	)

	namingCli, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		log.Printf("[%s] failed to create nacos naming client: %v, skipping", serviceName, err)
		return nil
	}

	ip := instanceIP()
	success, err := namingCli.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          ip,
		Port:        uint64(port),
		ServiceName: serviceName,
		Weight:      1.0,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata: map[string]string{
			"preserved.register.source": "GO_SDK",
		},
	})
	if err != nil || !success {
		log.Printf("[%s] nacos register failed (ip=%s:%d): %v, skipping", serviceName, ip, port, err)
		return nil
	}
	log.Printf("[%s] registered to nacos %s (ip=%s:%d)", serviceName, addr, ip, port)
	return namingCli
}

// DeRegister 从 Nacos 注销实例(优雅退出时调用)。
func DeRegister(serviceName string, port int, cli naming_client.INamingClient) {
	if cli == nil {
		return
	}
	ip := instanceIP()
	success, err := cli.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          ip,
		Port:        uint64(port),
		ServiceName: serviceName,
		Ephemeral:   true,
	})
	if err != nil || !success {
		log.Printf("[%s] nacos deregister failed: %v", serviceName, err)
		return
	}
	log.Printf("[%s] deregistered from nacos (ip=%s:%d)", serviceName, ip, port)
	// 等待 nacos 感知下线后再退出
	time.Sleep(time.Second)
}

// instanceIP 返回注册 IP。优先 NACOS_IP 环境变量,否则取 hostname 解析 IP。
func instanceIP() string {
	if ip := os.Getenv("NACOS_IP"); ip != "" {
		return ip
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "127.0.0.1"
	}
	addrs, err := net.LookupIP(hostname)
	if err != nil || len(addrs) == 0 {
		return "127.0.0.1"
	}
	return addrs[0].String()
}
