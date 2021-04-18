package server

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
	"github.com/hetznercloud/hcloud-go/hcloud"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
	"xorm.io/xorm"
)

type Server struct {
	DB                *xorm.Engine
	HetznerClient     *hcloud.Client
	HetznerSSHKeyName *string
	ExternalURI       *string
	GitStoragePath    *string
	cacheStore        *persistence.InMemoryStore
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

func (s *Server) NewRouter() *gin.Engine {
	router := gin.Default()

	s.cacheStore = persistence.NewInMemoryStore(time.Second)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/api/swagger/index.html")
	})

	api := router.Group("/api")
	api.Use(CORS())

	openapiURL := ginSwagger.URL("/api/swagger/doc.json")
	api.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, openapiURL))

	apiV1 := api.Group("/v1")
	apiV1.POST("/reportPackageModification", s.apiV1ReportPackageModification)

	workerV1 := apiV1.Group("/worker")
	workerV1.POST("/heartbeat/:hostname", s.apiV1WorkerHeartbeat)
	workerV1.GET("/requestWork", s.apiV1WorkerRequestWork)
	workerV1.PUT("/reportWorkResult", s.apiV1WorkerReportWorkResult)

	return router
}
