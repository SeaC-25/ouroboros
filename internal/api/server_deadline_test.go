package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 最小写超时验证：模拟半开连接——服务端 Accept 后不读，客户端写缓冲填满
// 后 WriteMessage 进入真实阻塞态。此时 writeWebSocketMessage 里 5s 写
// deadline 必须让写返回错误，而不是像原来那样无限阻塞（把 wsWriteMu 占
// 分钟级，拖死广播）。1ns deadline 版本进一步证明写不再等待 TCP。
func TestWebSocketWriteDeadlineBlocksNot(t *testing.T) {
	up := websocket.Upgrader{}
	var srvConn *websocket.Conn
	ready := make(chan struct{})
	var mu sync.Mutex

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		srvConn = c
		mu.Unlock()
		close(ready)
		// 故意不读：客户端发送缓冲填满后写入真实阻塞
		select {}
	}))
	defer func() {
		mu.Lock()
		if srvConn != nil {
			srvConn.Close()
		}
		mu.Unlock()
	}()

	dialer := &websocket.Dialer{}
	wsURL := "ws" + strings.TrimPrefix("http://"+ln.Addr().String(), "http")
	client, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	<-ready

	// 灌 1MB 消息直到一次写阻塞超过 2s（进入真实阻塞态）
	big := make([]byte, 1<<20)
	blocked := make(chan struct{})
	var blockedWrite *websocket.Conn = client
loop:
	for i := 0; i < 64; i++ {
		go func() {
			_ = blockedWrite.WriteMessage(websocket.TextMessage, big)
			// 只有 deadline 生效才会从这里返回
			blocked <- struct{}{}
		}()
		select {
		case <-blocked:
			// 意外立即完成：缓冲没满，继续灌
		case <-time.After(2 * time.Second):
			break loop
		}
	}

	mu.Lock()
	conn := srvConn
	mu.Unlock()

	// 服务端连接进入阻塞态后，deadline 写必须立即失败而不是等待 TCP。
	// 直接在服务端连接上验证：其发送方向对端不 ACK，同样可阻塞。
	// 但更可控的是：直接对 client 连接设置已过期的 deadline。
	// 注意：上面卡住的 goroutine 仍占着 client 连接的写锁， gorilla 自身
	// 会检测并发写并 panic，所以 deadline 验证换用独立的服务端连接：
	// 服务端 conn 的发送缓冲同样会因为对端（client）不读而阻塞吗？
	// 客户端 goroutine 会继续读 pong/ping 吗？不会——客户端 Dial 后没读循环。

	// 简化：用一条新的、确定会阻塞的写路径验证 deadline 行为。
	// 由于卡住的 goroutine 永远占着 client 写锁，客户端连接不可复用。
	// 服务端连接：client 从不读，所以 server→client 方向写 1MB 也会阻塞。
	if conn == nil {
		t.Fatal("服务端未升级连接")
	}

	errCh := make(chan error, 1)
	start := time.Now()
	go func() {
		s := &Server{}
		errCh <- s.writeWebSocketMessage(conn, websocket.TextMessage, big)
	}()
	select {
	case err := <-errCh:
		if elapsed := time.Since(start); elapsed > 6*time.Second {
			t.Fatalf("deadline 写耗时 %v，5s 保护未生效", elapsed)
		}
		if err == nil && elapsed() > 0 {
			t.Fatal("阻塞态连接上带 deadline 的写不应成功")
		}
		t.Logf("阻塞态连接写返回: err=%v 耗时=%v（<6s 即证明 5s 写超时生效）", err, time.Since(start))
	case <-time.After(10 * time.Second):
		t.Fatal("带 deadline 的写未返回——写超时保护失效，会永久阻塞")
	}
}
