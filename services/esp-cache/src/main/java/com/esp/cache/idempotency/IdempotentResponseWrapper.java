package com.esp.cache.idempotency;

import jakarta.servlet.ServletOutputStream;
import jakarta.servlet.WriteListener;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.servlet.http.HttpServletResponseWrapper;

import java.io.ByteArrayOutputStream;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;

/**
 * 捕获响应体的 HttpServletResponseWrapper(供 {@link IdempotencyFilter} 回写幂等缓存)。
 * 包装 getOutputStream/getWriter,所有写入同时落到捕获缓冲区与真实 response。
 */
public class IdempotentResponseWrapper extends HttpServletResponseWrapper {

    private final ByteArrayOutputStream capture = new ByteArrayOutputStream(1024);
    private ServletOutputStream output;
    private PrintWriter writer;
    private int status = 200;

    public IdempotentResponseWrapper(HttpServletResponse response) {
        super(response);
    }

    @Override
    public void setStatus(int sc) {
        super.setStatus(sc);
        this.status = sc;
    }

    // 注:Servlet 6 (Jakarta) 已移除 setStatus(int, String);此 override 仅记录状态码。
    @Override
    public int getStatus() {
        return status;
    }

    @Override
    public ServletOutputStream getOutputStream() {
        if (output == null) {
            output = new ServletOutputStream() {
                @Override
                public void write(int b) {
                    capture.write(b);
                    try { getResponse().getOutputStream().write(b); } catch (Exception ignored) {}
                }

                @Override
                public void write(byte[] b, int off, int len) {
                    capture.write(b, off, len);
                    try { getResponse().getOutputStream().write(b, off, len); } catch (Exception ignored) {}
                }

                @Override
                public boolean isReady() { return true; }

                @Override
                public void setWriteListener(WriteListener writeListener) {}
            };
        }
        return output;
    }

    @Override
    public PrintWriter getWriter() {
        if (writer == null) {
            writer = new PrintWriter(new OutputStreamWriter(getOutputStream(), StandardCharsets.UTF_8));
        }
        return writer;
    }

    @Override
    public void flushBuffer() {
        try { super.flushBuffer(); } catch (Exception ignored) {}
    }

    public byte[] getCapturedBody() {
        if (writer != null) writer.flush();
        return capture.toByteArray();
    }
}
