package event

import (
	"sync"
)

// Bus 是 session 级别的事件总线，支持多端同步。
// Agent 通过 Bus.Publish 发射事件，传输层通过 Subscribe 接收。
type Bus struct {
	mu   sync.RWMutex
	subs map[string]map[string]chan Event // sessionID -> subID -> ch
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string]map[string]chan Event)}
}

// Publish 向指定 session 的所有订阅者广播事件。
func (b *Bus) Publish(sessionID string, typ EventType, data any) {
	e := Event{SessionID: sessionID, Type: typ, Data: data}

	b.mu.RLock()
	subs := b.subs[sessionID]
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// 订阅者消费太慢时丢弃，不阻塞发布者
		}
	}
}

// Subscribe 注册一个 session 的事件订阅，返回的 unsub 函数用于取消订阅。
// chCap 控制 channel 缓冲大小。
func (b *Bus) Subscribe(sessionID string, chCap int) (ch <-chan Event, unsub func()) {
	c := make(chan Event, chCap)

	b.mu.Lock()
	if b.subs[sessionID] == nil {
		b.subs[sessionID] = make(map[string]chan Event)
	}
	id := nextID()
	b.subs[sessionID][id] = c
	b.mu.Unlock()

	unsub = func() {
		b.mu.Lock()
		delete(b.subs[sessionID], id)
		if len(b.subs[sessionID]) == 0 {
			delete(b.subs, sessionID)
		}
		b.mu.Unlock()
		close(c)
	}
	return c, unsub
}

var subIDCounter int

func nextID() string {
	subIDCounter++
	return intToStr(subIDCounter)
}

func intToStr(n int) string {
	const digits = "0123456789abcdef"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append(buf, digits[n%16])
		n /= 16
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
