package event

import "context"

type busKey struct{}

// WithBus 将 EventBus 存入 context，供 graph nodes 等深层调用发射事件。
func WithBus(ctx context.Context, b *Bus) context.Context {
	return context.WithValue(ctx, busKey{}, b)
}

// FromBus 从 context 中取出 EventBus。
func FromBus(ctx context.Context) *Bus {
	b, _ := ctx.Value(busKey{}).(*Bus)
	return b
}
