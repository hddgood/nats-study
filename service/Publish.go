package service

import (
	"fmt"
	"github.com/nats-io/nats.go"
	"log"
	"nats-study/dto"
	natsmiddleware "nats-study/middleware"
)

type Publish struct {
	Subject string
	Message string
}

func (p *Publish) PublishMessage() (err error) {
	var (
		js  nats.JetStreamContext
		ack *nats.PubAck
	)
	js = natsmiddleware.Js
	for i := 0; i < 10; i++ {
		ack, err = js.PublishMsg(&nats.Msg{
			Subject: p.Subject,
			Data:    []byte(p.Message),
			Header: nats.Header{
				"Nats-Expected-Last-Sequence": []string{fmt.Sprintf("%d", i)},
				"key":                         []string{"value"},
			},
		})
	}

	if err != nil {
		return err
	}

	log.Printf("Published message: %s %s ackSeq: %d", p.Subject, p.Message, ack.Sequence)
	return nil
}

func (p *Publish) ConsumeMessage(msg *nats.Msg) (err error) {
	var (
		subject string
		body    string
	)
	subject = msg.Subject
	body = string(msg.Data)
	fmt.Println("Received a message: ", subject, body)
	return
}

func FactoryPublishByDto(d *dto.PublishRequest) *Publish {
	return &Publish{
		Subject: d.Subject,
		Message: d.Message,
	}
}
