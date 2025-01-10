package v1

import (
	"github.com/gin-gonic/gin"
	"nats-study/dto"
	"nats-study/service"
)

func Ping(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"message": "pong",
	})
}

func Publish(ctx *gin.Context) {
	// 从post的body中获取json 数据
	// curl -X POST http://localhost:8080/api/v1/publish -d '{"message":"hello"}'
	req := &dto.PublishRequest{}
	err := ctx.BindJSON(req)
	if err != nil {
		ctx.JSON(400, gin.H{
			"message": "bad request",
		})
		return
	}
	publish := service.FactoryPublishByDto(req)
	err = publish.PublishMessage()
	if err != nil {
		ctx.JSON(500, gin.H{
			"message": "internal server error",
			"err":     err.Error(),
		})
		return
	}
}
