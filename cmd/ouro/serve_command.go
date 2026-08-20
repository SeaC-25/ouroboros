package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ouroboros/ouroboros/internal/api"
	"github.com/ouroboros/ouroboros/internal/app"
	"github.com/spf13/cobra"
)

func newServeCommand(out io.Writer) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve <workspace>",
		Short: "启动HTTP API服务器",
		Long:  "启动HTTP API服务器，为GUI前端提供数据接口",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := args[0]

			// 适配任意工作区：如果目录还不是有效的 Ouroboros 工作区，
			// 自动补齐必需文件与目录（幂等，对已有工作区安全）。
			if err := app.InitWorkspace(workspace); err != nil {
				return fmt.Errorf("初始化工作区失败: %w", err)
			}

			// 创建API服务器
			server, err := api.NewServer(workspace, port)
			if err != nil {
				return fmt.Errorf("创建API服务器失败: %w", err)
			}

			// 启动服务器（在goroutine中）
			errChan := make(chan error, 1)
			go func() {
				if err := server.Start(); err != nil {
					errChan <- err
				}
			}()

			// 等待中断信号
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			select {
			case err := <-errChan:
				return fmt.Errorf("服务器错误: %w", err)
			case <-sigChan:
				fmt.Fprintf(out, "\n收到中断信号，正在关闭服务器...\n")
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return server.Stop(ctx)
			}
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "HTTP服务器端口")
	return cmd
}
