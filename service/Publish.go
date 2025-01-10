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
		natsConn *nats.Conn
	)
	natsConn = natsmiddleware.NatsConn
	err = natsConn.Publish(p.Subject, []byte(p.Message))
	if err != nil {
		return err
	}
	//for i := 0; i < 10; i++ {
	//	data := "idx: " + fmt.Sprintf("%d", i) + " " + p.Message
	//	err = natsConn.PublishMsg(&nats.Msg{
	//		Header:  map[string][]string{},
	//		Subject: p.Subject,
	//		Data:    []byte(data),
	//		Reply:   "ok",
	//	})
	//	if err != nil {
	//		return err
	//	}
	//}

	log.Println("Published message: ", p.Subject, p.Message)
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
