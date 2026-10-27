package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWebSocketWriteTimeoutFailsNotForever 验证 5s 写超时的两个关键性质：
//
//  1. 机制层（确定性）：已过期的 deadline 让 WriteMessage 立即返回
//     i/o timeout 而非阻塞 —— 这是 writeWebSocketMessage 修复依赖的
//     底层机制（gorilla 每帧写前检查 writeDeadline）。
//  2. 集成层（行为）：writeWebSocketMessage 在写前设置 5s deadline，
//     因此对一条写不进去的连接，调用必须在 ~5s 后返回错误，而不是
//     永久阻塞把 wsWriteMu 占住分钟级。
//
// "写不进去" 的构造：客户端 socket 接收缓冲压到 4KB 且升级后永不读，
// 服务端写入很快填满内核缓冲进入真实 TCP 阻塞（独立探针已证实：
// 64MB 累计写后 >3s 无进展且 30s 不恢复）。

// TestLimitRequestBody 验证 limitRequestBody：超限 JSON 被拒（413 语义），
// 正常请求不受影响。
func TestLimitRequestBody(t *testing.T) {
	const max = 1 << 20 // 1MB，测试用小上限
	srv := httptest.NewServer(limitRequestBody(max, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// MaxBytesReader 超限时 Decode 报错；handler 映射为 413
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
