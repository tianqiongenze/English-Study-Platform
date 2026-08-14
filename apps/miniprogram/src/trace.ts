// W3C traceparent 生成器(断点6):小程序请求注入 traceparent,使 BFF EdgeAuthHandler
// extract 上游 context 而非自建根 span,实现 前端→BFF→下游→Tempo 端到端 trace 贯通。
//
// 轻量方案:不上完整 OTel SDK(小程序包体限制),仅生成 W3C traceparent header 注入。
// 格式:version(00)-traceId(32hex)-spanId(16hex)-flags(01 sampled)。
// traceId/spanId 用 wx.getRandomValues 生成(微信小程序环境);低版本兜底 Math.random。

function randomHex(bytes: number): string {
  const arr = new Array(bytes);
  for (let i = 0; i < bytes; i++) {
    arr[i] = Math.floor(Math.random() * 256);
  }
  return arr.map((b) => b.toString(16).padStart(2, "0")).join("");
}

export function traceparent(): string {
  // 优先用 wx.getRandomValues(同步,微信基础库 2.26.0+);否则降级 Math.random。
  try {
    const buf = new Uint8Array(16);
    // wx 已有全局类型声明(无需 @ts-expect-error)
    if (typeof wx !== "undefined" && wx.getRandomValues) {
      wx.getRandomValues({
        length: 16,
        success: (res: { randomValues: ArrayBuffer }) => {
          const view = new Uint8Array(res.randomValues);
          for (let i = 0; i < 16; i++) buf[i] = view[i];
        },
      });
    }
  } catch {
    // 降级走 randomHex
  }
  let traceId = randomHex(16);
  let spanId = randomHex(8);
  if (traceId === "00000000000000000000000000000000") traceId = randomHex(16);
  if (spanId === "0000000000000000") spanId = randomHex(8);
  return `00-${traceId}-${spanId}-01`;
}
