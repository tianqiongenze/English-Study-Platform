//go:build !pulsar_eda

// rankings_projector_stub.go —— Pulsar CQRS 投影器 stub(偏差①)。
//
// 默认编译(无 pulsar_eda tag)时 no-op,保持 `go build ./src` 不拉 pulsar-client-go 依赖。
// 启用真实投影器:`go build -tags pulsar_eda ./src`(此时 rankings_projector.go 参与编译,本文件排除)。
// pollLoop 始终在跑作完整源兜底(坑㉙:content 事件不走 Pulsar),故 stub 模式下排行喂数不缺失。
package main

// startRankingsProjector 默认 no-op:未启用 pulsar_eda 时轮询 pollLoop 是唯一喂数源。
func (s *Server) startRankingsProjector() {}
