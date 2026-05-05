package main

import (
	"context"

	agentcontext "github.com/agent-pilot/agent-pilot-be/agent/context"
	"github.com/agent-pilot/agent-pilot-be/agent/graph"
	"github.com/agent-pilot/agent-pilot-be/agent/graph/nodes"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/agent-pilot/agent-pilot-be/agent/react"
	"github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	"github.com/agent-pilot/agent-pilot-be/config"
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	authService "github.com/agent-pilot/agent-pilot-be/controller/auth/service"
	"github.com/agent-pilot/agent-pilot-be/controller/chat"
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/ioc"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/agent-pilot/agent-pilot-be/server"
)

// 不引入wire了，wire有时候还是太吃屎了，中小型还是自己维护比较快
func initWebServer() *App {
	conf, err := config.LoadFromEnv()
	if err != nil {
		panic(err)
	}
	// ioc
	logger := ioc.InitLogger(conf.Logconf)
	om := ioc.NewOpenAIModelClient(context.Background(),
		conf.OpenAIModel, conf.OpenAIBaseURL, conf.OpenAIAPIKey)

	// 加载 skills
	skillDir := "skills"
	skillReg, _ := skill.LoadSkills(skillDir)

	// 构建 tools
	tools := tool.BuildTools(skillReg)

	// 构建 system prompt
	systemMsg := chat.BuildSystemPrompt(skillReg.List())

	mongoDB := ioc.InitMongoDatabase(conf.MongoDBUri, conf.MongoDBDatabase)
	agentDao := dao.NewAgentDao(mongoDB)
	memService := memory.NewMemoryService(agentDao)
	planner := agentplan.NewLLMPlanner(om.Model, skillReg)
	ctxBuilder := agentcontext.NewBuilder(agentcontext.Options{
		SystemPrompt: systemMsg,
		MaxMessages:  20,
	})
	executor := react.NewExecutor(om.Model, tools, ctxBuilder)

	schNode := nodes.NewSchedulerNode()
	plannerNode := nodes.NewPlannerNode(memService, planner, ctxBuilder)
	executorNode := nodes.NewExecutorNode(memService, executor)
	agentGraph := graph.NewAgentGraph(schNode, plannerNode, executorNode, memService)

	cc := chat.NewController(context.Background(), skillReg, systemMsg, memService, agentGraph)

	// Agent 核心服务（传输层无关，供 WS 等后续使用）

	//pkg
	redisJWTHandler := jwt.NewRedisJWTHandler(conf.JwtConf)
	// middleware
	authM := middleware.NewAuthMiddleware(redisJWTHandler)
	corM := middleware.NewCorsMiddleware(conf.CorMiddlewareConf)
	logM := middleware.NewLoggerMiddleware(logger)
	// service
	larkSvc := authService.NewLarkService()
	// controller
	hc := health.NewHealthController()
	authC := auth.NewLarkAuthController(
		conf.FeishuAppID,
		conf.FeishuAppSecret,
		conf.FeishuRedirectURI,
		conf.StateSecret,
		larkSvc,
		redisJWTHandler,
	)
	srv := server.NewServer(hc, authC, cc, authM, corM, logM)
	return NewApp(srv, &conf)
}
