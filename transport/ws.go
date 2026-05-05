package transport

import (
	"context"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/event"
	"golang.org/x/net/websocket"
)

// WSMsg 是客户端和服务器之间传递的消息结构。
type WSMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

// WSEvent 是服务器推送给客户端的消息结构。
type WSEvent struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	Data    any    `json:"data,omitempty"`
}

// Handler 桥接 WebSocket ↔ EventBus。
// 每个 session 可以有多个 WebSocket 连接，事件会广播给所有同 session 的连接。
type Handler struct {
	bus       *event.Bus
	onMessage func(ctx context.Context, sessionID string, msg WSMsg)
}

// NewHandler 创建 WS handler。onMessage 在收到客户端消息时回调。
func NewHandler(bus *event.Bus, onMessage func(ctx context.Context, sessionID string, msg WSMsg)) *Handler {
	return &Handler{
		bus:       bus,
		onMessage: onMessage,
	}
}

// ServeWS 处理一个 WebSocket 连接。
func (h *Handler) ServeWS(conn *websocket.Conn) {
	sessionID := conn.Request().URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = "default"
	}

	// 订阅 event bus
	evCh, unsub := h.bus.Subscribe(sessionID, 64)

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			unsub()
		})
	}

	// 读协程：WS → onMessage
	go func() {
		defer cleanup()
		for {
			var msg WSMsg
			if err := websocket.JSON.Receive(conn, &msg); err != nil {
				return
			}
			if h.onMessage != nil {
				h.onMessage(context.Background(), sessionID, msg)
			}
		}
	}()

	// 写协程：event bus → WS
	defer cleanup()
	for e := range evCh {
		evt := WSEvent{
			Type:    string(e.Type),
			Session: sessionID,
			Data:    e.Data,
		}
		if err := websocket.JSON.Send(conn, evt); err != nil {
			return
		}
	}
}
