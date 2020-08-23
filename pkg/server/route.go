package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/server/handler"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
)

// setup the gin http server handler

func setupRoute(kubeClient client.Client) http.Handler {
	// create the router
	router := gin.New()
	loggerConfig := gin.LoggerConfig{
		Output: os.Stdout,
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("%v | %3d | %13v | %15s | %-7s %s | %s\n",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
				param.ErrorMessage,
			)
		},
	}
	router.Use(gin.LoggerWithConfig(loggerConfig))
	router.Use(util.SetRequestID())
	router.Use(util.SetContext())
	router.Use(gin.Recovery())
	router.Use(util.ValidateHeaders())
	//Store kubernetes client which could be retrieved by handlers
	router.Use(util.StoreClient(kubeClient))
	// all requests start with /api
	api := router.Group(util.RootPath)
	// env related operation
	envs := api.Group(util.EnvironmentPath)
	{
		envs.POST("/", handler.CreateEnv)
		envs.GET("/:envName", handler.GetEnv)
		envs.GET("/", handler.ListEnv)
		envs.DELETE("/:envName", handler.DeleteEnv)
		envs.PATCH("/:envName", handler.SwitchEnv)
		// app related operation
		apps := envs.Group("/:envName/apps")
		{
			//apps.POST("/", handler.CreateApps)
			apps.GET("/:appName", handler.GetApp)
			apps.PUT("/:appName", handler.UpdateApps)
			apps.GET("/", handler.ListApps)
			apps.DELETE("/:appName", handler.DeleteApps)

			traitWorkload := apps.Group("/:appName/" + util.TraitDefinitionPath)
			{
				traitWorkload.POST("/", handler.AttachTrait)
				traitWorkload.DELETE("/:traitName", handler.DetachTrait)
			}
		}
	}
	// workload related api
	workload := api.Group(util.WorkloadDefinitionPath)
	{
		workload.POST("/", handler.CreateWorkload)
		workload.GET("/:workloadName", handler.GetWorkload)
		workload.PUT("/:workloadName", handler.UpdateWorkload)
		workload.GET("/", handler.ListWorkload)
	}
	// trait related api
	trait := api.Group(util.TraitDefinitionPath)
	{
		trait.GET("/:traitName", handler.GetTrait)
		trait.GET("/", handler.ListTrait)
	}
	// scope related api
	scopes := api.Group(util.ScopeDefinitionPath)
	{
		scopes.POST("/", handler.CreateScope)
		scopes.GET("/:scopeName", handler.GetScope)
		scopes.PUT("/:scopeName", handler.UpdateScope)
		scopes.GET("/", handler.ListScope)
		scopes.DELETE("/:scopeName", handler.DeleteScope)
	}

	// scope related api
	repo := api.Group(util.RepoPath)
	{
		repo.GET("/:categoryName/:definitionName", handler.GetDefinition)
		repo.PUT("/:categoryName/:definitionName", handler.UpdateDefinition)
		repo.GET("/:categoryName", handler.ListDefinition)
	}
	// version
	api.GET(util.VersionPath, handler.GetVersion)
	// default
	router.NoRoute(util.NoRoute())

	return router
}
