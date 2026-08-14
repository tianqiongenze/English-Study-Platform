//go:build !pulsar_eda

package pulsar

// PublishSessionEnded 默认 no-op(未启用 pulsar_eda build tag 时)。
//
// 网络受限/本地/CI 环境不连 Pulsar,默认编译此 stub,保持 `go build ./...` 通过。
// 启用真实 producer:`go get github.com/apache/pulsar-client-go` 后
// `go build -tags pulsar_eda ./...`(此时 producer.go 参与编译,本文件排除)。
func PublishSessionEnded(payload map[string]any) {}
