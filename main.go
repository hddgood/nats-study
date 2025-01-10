package main

import (
	"nats-study/consumer"
	"nats-study/middleware"
	"nats-study/router"
)

func main() {
	middleware.InitNatsConn()

	consumer.InitConsumer()

	engine := router.InitRouter()
	engine.Run(":8081")
}
