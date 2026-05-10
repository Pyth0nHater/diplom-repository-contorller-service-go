package http

import (
	"repository-contorller-service-go/internal/controller"
	"repository-contorller-service-go/internal/middleware"
	"repository-contorller-service-go/internal/service"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewRouter(
	authController *controller.AuthController,
	repoController *controller.RepositoryController,
	deploymentController *controller.DeploymentController,
	webhookController *controller.WebhookController,
	authService *service.AuthService,
) *gin.Engine {
	router := gin.Default()

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := router.Group("/api/v1")
	authGroup := api.Group("/auth")
	authGroup.POST("/register", authController.Register)
	authGroup.POST("/login", authController.Login)
	authGroup.GET("/me", middleware.AuthMiddleware(authService), authController.Me)
	authGroup.PUT("/github-token", middleware.AuthMiddleware(authService), authController.SaveGitHubToken)
	authGroup.GET("/github-token", middleware.AuthMiddleware(authService), authController.GetGitHubToken)
	authGroup.DELETE("/github-token", middleware.AuthMiddleware(authService), authController.DeleteGitHubToken)

	repoGroup := api.Group("/repositories", middleware.AuthMiddleware(authService))
	repoGroup.POST("", repoController.Create)
	repoGroup.GET("", repoController.List)
	repoGroup.GET("/:id", repoController.Get)
	repoGroup.PUT("/:id", repoController.Update)
	repoGroup.DELETE("/:id", repoController.Delete)
	repoGroup.POST("/:id/deploy", deploymentController.Deploy)
	repoGroup.POST("/:id/bootstrap", deploymentController.BootstrapRepository)

	api.POST("/webhook/github", webhookController.GithubPush)
	api.POST("/webhook/deploy", webhookController.DeployByToken)

	return router
}
