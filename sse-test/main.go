// mockclient.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============ 模拟客户端 ============

type MockClient struct {
	baseURL  string
	client   *http.Client
	lastID   string // 记录最近收到的 id，用于重连续传
	mu       sync.Mutex
	received []string // 收到的事件摘要，用于断言
}

func NewMockClient(baseURL string) *MockClient {
	return &MockClient{
		baseURL: baseURL,
		client: &http.Client{
			// 不设 Timeout，长连接；靠 context 控制
			Transport: &http.Transport{
				DisableCompression: true,
			},
		},
	}
}

func (c *MockClient) setLastID(id string) {
	c.mu.Lock()
	c.lastID = id
	c.mu.Unlock()
}

func (c *MockClient) getLastID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastID
}

// Connect 发起一次 SSE 请求；onLine 对每一行原始数据回调（含注释行）
// 返回时表示连接结束
func (c *MockClient) Connect(ctx context.Context, sessionID string, onLine func(string)) error {
	url := c.baseURL + "/events/" + sessionID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if lid := c.getLastID(); lid != "" {
		req.Header.Set("Last-Event-ID", lid)
		fmt.Printf("[client] 重连携带 Last-Event-ID=%s\n", lid)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("[client] 连接建立 status=%d, 耗时=%v\n", resp.StatusCode, time.Since(start))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	reader := bufio.NewReader(resp.Body)
	var dataLines []string
	var curEvent string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("[client] 服务端关闭了连接 (EOF)")
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if onLine != nil {
			onLine(line)
		}

		// 解析，维护 lastID
		switch {
		case line == "":
			if curEvent == "session_closed" {
				return nil
			}
			curEvent = ""
			dataLines = dataLines[:0]
		case strings.HasPrefix(line, ":"):
			// 心跳/注释
		case strings.HasPrefix(line, "id:"):
			c.setLastID(strings.TrimSpace(line[3:]))
		case strings.HasPrefix(line, "event:"):
			curEvent = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
		}
	}
}
