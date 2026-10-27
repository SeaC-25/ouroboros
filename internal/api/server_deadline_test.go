package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 最小写超时验证：给一条会阻塞的 WS 连接设 1ns deadline，
// WriteMessage 必须立即返回错误（i/o timeout），而非永久阻塞。
// 这证明 writeWebSocketMessage 里的 SetWriteDeadline 能把半开连接的
// "锁占分钟级" 压成最多 5s。
func TestWebSocketWriteDeadlineBlocksNot(t *testing.T) {
	up := websocket.Upgrader{}
	var srvConn *websocket.Conn
	ready := make(chan struct{})

	// 自建 listener，服务端 Accept 后持有连接但不读——客户端写缓冲填满后
	// WriteMessage 会一直阻塞，模拟半开/对端不收包的真实场景。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		httpSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			srvConn = c
			close(ready)
			// 故意不读：让客户端发送缓冲最终填满
			select {}
		})}
		httpSrv.Serve(ln)
	}()

	dialer := &websocket.Dialer{}
	wsURL := "ws" + strings.TrimPrefix("http://"+ln.Addr().String(), "http")
	client, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	<-ready
	defer srvConn.Close()

	// 灌大消息直到一次写阻塞超过 2s（缓冲填满，进入真实阻塞态）
	big := make([]byte, 1<<20) // 1MB
	blocked := false
	for i := 0; i < 64 && !blocked; i++ {
		done := make(chan error, 1)
		go func() { done <- client.WriteMessage(websocket.TextMessage, big) }()
		select {
		case <-done:
			// 本条写进缓冲了，继续灌
		case <-time.After(2 * time.Second):
			blocked = true
		}
	}
	if !blocked {
		t.Skip("写缓冲未能填满（环境缓冲过大），跳过阻塞态验证")
	}

	// 关键验证：1ns deadline 下 writeWebSocketMessage 必须立即返回错误
	s := &Server{}
	start := time.Now()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.writeWebSocketMessage(client, websocket.TextMessage, big)
	}()
	select {
	case err := <-errCh:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("带 deadline 的写仍耗时 %v，未及时返回", elapsed)
		}
		if err == nil {
			t.Fatal("过期 deadline 下 WriteMessage 应返回错误，却成功了")
		}
		t.Logf("1ns deadline 写立即失败: %v (耗时 %v) —— 写超时生效", err, time.Since(start))
	case <-time.After(5 * time.Second):
		t.Fatal("带 deadline 的写未在 5s 内返回——超时保护失效，会永久阻塞")
	}
}

// 验证 limitRequestBody：超过上限的 JSON 请求被 413 拒绝，正常请求不受影响。
func TestLimitRequestBody(t *testing.T) {
	const max = 1 << 20 // 1MB，测试用小上限
	srv := httptest.NewServer(limitRequestBody(max, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// MaxBytesReader 超限时 Decode 报错；直接映射 413
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})))
	defer srv.Close()

	// 超限请求 → 拒绝
	oversized := strings.Repeat("a", max+1024)
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"data":"`+oversized+`"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限请求应 413，得 %d", resp.StatusCode)
	}

	// 正常请求 → 通过
	resp, err = http.Post(srv.URL, "application/json", strings.NewReader(`{"ok":true}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("正常请求应 200，得 %d", resp.StatusCode)
	}
}
