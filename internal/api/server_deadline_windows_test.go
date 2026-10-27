//go:build windows

package api

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketWriteTimeoutFailsNotForever(t *testing.T) {
	up := websocket.Upgrader{}
	srvReady := make(chan *websocket.Conn, 1)
	park := make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		srvReady <- c
		<-park // 升级后不读不写，模拟半开/不收包对端
	}))

	// 客户端 socket：4KB 接收缓冲 + 永不读
	shrinkRecvBuf := func(network, address string, c syscall.RawConn) error {
		var serr error
		_ = c.Control(func(fd uintptr) {
			serr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4096)
		})
		return serr
	}
	dialer := &websocket.Dialer{NetDial: func(network, addr string) (net.Conn, error) {
		d := net.Dialer{Control: shrinkRecvBuf}
		return d.DialContext(context.Background(), network, addr)
	}}
	cl, _, err := dialer.Dial("ws://"+ln.Addr().String(), nil)
	if err != nil {
		ln.Close()
		close(park)
		t.Fatalf("dial: %v", err)
	}
	sc := <-srvReady
	cleanup := func() { cl.Close(); sc.Close(); close(park); ln.Close() }
	defer cleanup()

	s := &Server{}

	// --- 性质 1：过期 deadline 立即失败（机制验证，确定性） ---
	if err := cl.SetWriteDeadline(time.Now().Add(time.Nanosecond)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	start := time.Now()
	err = cl.WriteMessage(websocket.TextMessage, []byte("probe"))
	if err == nil {
		t.Fatal("过期 deadline 下 WriteMessage 应返回错误，却成功了")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("过期 deadline 下写耗时 %v，未立即失败", elapsed)
	}
	t.Logf("性质1 过期deadline写立即失败: %v（%v）", err, time.Since(start))

	// --- 性质 2：writeWebSocketMessage 的 5s deadline 释放阻塞写 ---
	// 探针已实测：对端 4KB 接收缓冲且不读时，回环内核缓冲能吸收首条
	// 64MB（写成功），第二条 64MB 必然进入真实阻塞态且 30s 不恢复；
	// "阻塞中再设 deadline" 无法唤醒进行中的帧（gorilla 只在帧开始时
	// 应用 deadline），但 writeWebSocketMessage 是写前设 deadline，
	// 阻塞写必然在 ~5s 被 i/o timeout 释放。因此两相串行：
	//   相1：普通写（自带 5s 护栏）灌满内核缓冲；
	//   相2：writeWebSocketMessage 写，必须以 deadline 错误 ~5s 收场。
	big := make([]byte, 64<<20)

	fillDone := make(chan error, 1)
	go func() {
		_ = sc.SetWriteDeadline(time.Now().Add(5 * time.Second)) // 护栏防测试自身卡死
		fillDone <- sc.WriteMessage(websocket.TextMessage, big)
	}()
	select {
	case err := <-fillDone:
		t.Logf("相1 缓冲灌充写返回: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("相1 灌充写 8s 未返回（4KB 对端不该吸收首条 64MB）")
	}

	errCh := make(chan error, 1)
	start = time.Now()
	go func() { errCh <- s.writeWebSocketMessage(sc, websocket.TextMessage, big) }()

	select {
	case err := <-errCh:
		elapsed := time.Since(start)
		if err == nil {
			// 极小概率：第二条 64MB 也被吸收。不判失败，但说明本次
			// 未进入阻塞态，deadline 释放路径未被触发。
			t.Skipf("相2 64MB 写被吸收（%v），本次未进入阻塞态", elapsed)
		}
		if elapsed < 3*time.Second || elapsed > 8*time.Second {
			t.Fatalf("相2 写经 %v 返回（err=%v），期望 ~5s 由 deadline 释放", elapsed, err)
		}
		t.Logf("性质2 阻塞写被 5s deadline 释放: %v（%v）——修复前此写会永久阻塞", err, elapsed)
	case <-time.After(10 * time.Second):
		t.Fatal("带 deadline 的写 10s 内未返回——写超时保护失效（修复前会永久阻塞）")
	}
}
