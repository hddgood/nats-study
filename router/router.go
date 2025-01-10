package router

import (
	"github.com/gin-gonic/gin"
	v1 "nats-study/router/api/v1"
)

func InitRouter() *gin.Engine {
	r := gin.New()

	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	apiV1 := r.Group("/api/v1")
	{
		apiV1.GET("/ping", v1.Ping)
		apiV1.POST("/publish", v1.Publish)
	}

	return r
}
