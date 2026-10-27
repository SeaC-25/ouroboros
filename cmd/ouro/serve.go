package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
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
		Short: "启动 Ouroboros HTTP API 服务器，为桌面 GUI 提供 RESTful 接口",
		Long: `serve 命令启动一个 HTTP API 服务器，监听指定端口。
桌面 Electron 应用会自动连接到此服务器获取工作区数据。

示例:
  ouro serve /path/to/workspace --port 8080
  ouro serve C:\Workspaces\my-pentest -p 9000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace := args[0]

			// 验证工作区路径
			if _, err := os.Stat(workspace); os.IsNotExist(err) {
				return fmt.Errorf("工作区路径不存在: %s", workspace)
			}

			// 适配任意工作区：目录还不是有效的 Ouroboros 工作区时自动
			// 补齐必需文件与目录（幂等；replay/events.db 等由 API 服务
			// 依赖，缺失会让 SQLite 以 unable to open database file 失败）。
			if err := app.InitWorkspace(workspace); err != nil {
				return fmt.Errorf("初始化工作区失败: %w", err)
			}

			// 创建 API 服务器
			server, err := api.NewServer(workspace, port)
			if err != nil {
				return fmt.Errorf("创建 API 服务器失败: %w", err)
			}

			// 在 goroutine 中启动服务器
			serverErr := make(chan error, 1)
			go func() {
				if err := server.Start(); err != nil && err != http.ErrServerClosed {
					serverErr <- err
				}
			}()

			// 等待启动完成（给服务器 1 秒时间绑定端口）
			time.Sleep(1 * time.Second)

			// 检查是否有启动错误
			select {
			case err := <-serverErr:
				return fmt.Errorf("服务器启动失败: %w", err)
			default:
				// 服务器启动成功
			}

			log.Printf("✓ Ouroboros API 服务器已启动")
			log.Printf("  工作区: %s", workspace)
			log.Printf("  监听地址: http://127.0.0.1:%d", port)
			log.Printf("  健康检查: http://127.0.0.1:%d/api/health", port)
			log.Printf("  GUI 引导: http://127.0.0.1:%d/api/gui/bootstrap", port)
			log.Printf("\n按 Ctrl+C 停止服务器\n")

			// 等待中断信号
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

			select {
			case err := <-serverErr:
				return fmt.Errorf("服务器运行时错误: %w", err)
			case sig := <-sigChan:
				log.Printf("\n收到信号 %v，正在关闭服务器...", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				if err := server.Stop(ctx); err != nil {
					log.Printf("警告: 服务器关闭时出现错误: %v", err)
					return err
				}

				log.Println("✓ 服务器已优雅关闭")
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP 服务器监听端口（默认 8080）")
	return cmd
}
