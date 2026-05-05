package router

import (
	"github.com/agent-pilot/agent-pilot-be/controller/chat"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func registerChat(s *gin.RouterGroup, l *middleware.LoggerMiddleware, authMiddleware *middleware.AuthMiddleware, c *chat.Controller) {
	chatGroup := s.Group("/chat")
	chatGroup.Use(authMiddleware.MiddlewareFunc())
	chatGroup.Use(l.NormalMiddlewareFunc())
	// 流式响应不使用普通日志中间件，因为它会缓冲整个响应
	// 可以在 controller 内部自行记录日志
	//chatGroup.POST("/stream", c.Chat)
	chatGroup.POST("/sync", ginx.WrapReq(c.ChatSync))
	chatGroup.POST("/session", ginx.WrapClaims(c.CreateSession))
	//chatGroup.POST("/plan", c.Plan)
	//chatGroup.POST("/execute", c.Execute)

	// WS 单独注册（无 auth，使用 session_id 作为会话标识）
	s.GET("/chat/ws", c.ChatWS)
}
