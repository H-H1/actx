// Command actx-web 将 ACTX 代理作为 HTTP SSE 服务器启动。
// 它不依赖 CLI/TUI，通过 REST API 提交操作、通过 SSE 接收事件流。
//
// 用法：
//
//	go run ./cmd/actx-web --addr 127.0.0.1:8080
//
// 然后浏览器打开 http://127.0.0.1:8080 使用内嵌测试页面，
// 或直接调用 REST API：
//
//	POST /api/sessions                 创建会话
//	GET  /api/sessions/{id}/events     SSE 事件流
//	POST /api/sessions/{id}/message    发送消息
//	POST /api/sessions/{id}/interrupt  中止当前轮次
//	POST /api/sessions/{id}/approval   响应审批请求
//	POST /api/sessions/{id}/compact    压缩上下文
//	DELETE /api/sessions/{id}          关闭会话
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/login"
	"github.com/H-H1/actx/internal/sseweb"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8088", "HTTP 监听地址")
	baseURL := flag.String("base-url", "", "覆盖模型 API 基础 URL")
	model := flag.String("model", "", "覆盖默认模型")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// 加载基础配置：所有会话在此配置上派生。
	cfg, err := config.LoadConfig(config.ACTXHomeDir(), nil)
	if err != nil {
		fatal(logger, "load config: %v", err)
	}
	if *model != "" {
		cfg.Model = *model
	}

	// 获取 API 密钥：环境变量 → auth.json → .actx/config.json。
	apiKey, err := login.GetAPIKey(cfg.ACTXHome)
	if err != nil {
		fatal(logger, "%v", err)
	}

	if *baseURL == "" {
		*baseURL = cfg.BaseURL
	}

	srv := sseweb.New(cfg, apiKey, *baseURL, logger)
	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
	}

	// 信号处理：SIGINT/SIGTERM 时优雅关闭服务器与所有会话。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		srv.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	logger.Info("ACTX web server listening",
		"addr", *addr, "model", cfg.Model, "provider", cfg.ModelProvider)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal(logger, "server: %v", err)
	}
}

// fatal 输出错误并退出。
func fatal(logger *slog.Logger, format string, args ...any) {
	logger.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
