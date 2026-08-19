// LCO 代理入口:LLM 多级缓存铁律(LCO)落地——给本机 Claude Code/ZCode 等工具接入三层缓存。
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lco-proxy/internal/cache"
	"lco-proxy/internal/config"
	"lco-proxy/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("配置加载失败", "err", err)
		os.Exit(1)
	}
	logger.Info("LCO 代理启动", "listen", cfg.Listen, "upstream", cfg.UpstreamURL,
		"l1_redis", cfg.RedisAddr, "l2", cfg.L2Enabled, "l3", cfg.L3Enabled)

	cm, err := cache.New(cfg, logger)
	if err != nil {
		logger.Error("缓存管理器初始化失败", "err", err)
		os.Exit(1)
	}

	srv := server.New(cfg, cm, logger)
	httpSrv := &http.Server{
		Addr:    cfg.Listen,
		Handler: srv.Routes(),
		// LLM 长尾,读超时放宽
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("LCO 代理监听中", "addr", cfg.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("LCO 代理关闭中")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
