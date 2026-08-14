// W3C traceparent 生成器(断点6):前端请求注入 traceparent,使 BFF EdgeAuthHandler
// extract 上游 context 而非自建根 span,实现 前端→BFF→下游→Tempo 端到端 trace 贯通。
//
// 轻量方案:不上完整 OTel SDK(前端 bundle 重 + 性能负担),仅生成 W3C traceparent
// header 注入。格式:version(00)-traceId(32hex)-spanId(16hex)-flags(01 sampled)。
// traceId/spanId 用 crypto.getRandomValues 生成(浏览器环境)。

function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function traceparent(): string {
  // traceId: 16 字节(32 hex);全 0 非法,重生成。
  let traceId = randomHex(16);
  if (traceId === "00000000000000000000000000000000") traceId = randomHex(16);
  // spanId: 8 字节(16 hex);全 0 非法,重生成。
  let spanId = randomHex(8);
  if (spanId === "0000000000000000") spanId = randomHex(8);
  return `00-${traceId}-${spanId}-01`;
}
