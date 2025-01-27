package middleware

import (
	"github.com/nats-io/nats.go"
)

var NatsConn *nats.Conn

var Js nats.JetStreamContext

func InitNatsConn() {
	connect, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		panic(err)
	}

	NatsConn = connect

	Js, err = NatsConn.JetStream()
	if err != nil {
		panic(err)
	}
}
