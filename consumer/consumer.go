package consumer

import (
	"context"
	"fmt"
	"github.com/nats-io/nats.go"
	"log"
	"nats-study/middleware"
	"nats-study/service"
	"sync"
	"time"
)

var Subject = "order.test"

var streamName = "order-stream"

var SubjectFuncMap = map[string]func(msg *nats.Msg) error{}

func InitConsumer() {
	log.Println("InitConsumer start")
	js, err := middleware.NatsConn.JetStream()
	if err != nil {
		log.Panic(err)
	}

	log.Println("Init JetStream success")

	SubjectFuncMap[Subject] = testServiceConsume

	// 判斷Stream是否存在，如果不存在，那麼需要建立這個Stream，否則會導致pub/sub失敗
	stream, err := js.StreamInfo(streamName)
	if err != nil {
		log.Println(err) // 如果不存在，這裡會有報錯
	}
	if stream == nil {
		log.Printf("creating stream %q and subject %q", streamName, Subject)
		stream, err = js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{Subject},
			MaxAge:   3 * 24 * time.Hour,
			Storage:  nats.FileStorage,
			Replicas: 3,
		})
		if err != nil {
			log.Panicln(err)
		}
	}

	// PUSH
	//_, err = js.Subscribe(Subject, handlePushMessage, nats.Durable("order-consume-push-test-2"), nats.ManualAck())
	//if err != nil {
	//	log.Panic(err)
	//	return
	//}

	// PUSH QUEUE
	//_, err = js.QueueSubscribe(Subject, "order-consume-push-queue-test", handlePushMessage, nats.ManualAck())
	//if err != nil {
	//	log.Panic(err)
	//	return
	//}

	// PULL
	sub, err := js.PullSubscribe(Subject, "order-consume-pull-test", nats.ManualAck())
	if err != nil {
		log.Panic(err)
		return
	}

	log.Println("PullSubscribe success")

	// 消费消息，处理拉取的消息
	go ConsumeNats(sub)
}

func ConsumeNats(sub *nats.Subscription) {
	var (
		err  error
		msgs []*nats.Msg
	)
	log.Println("Start to consume messages")
	for {
		// 每次拉取 5 条消息
		msgs, err = sub.Fetch(1)
		if err != nil {
			log.Printf("Error fetching messages: %v", err)
			time.Sleep(time.Second)
			continue
		}

		// 处理拉取到的每条消息
		var wg sync.WaitGroup
		for _, msg := range msgs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// 处理消息
				err := handleMessage(msg)
				if err != nil {
					log.Printf("Error handling message: %v", err)
				}

				// 确认消息已处理
				err = msg.Ack()
				if err != nil {
					log.Printf("Error acknowledging message: %v", err)
				}
			}()
		}
		wg.Wait()
	}
}

func handlePushMessage(msg *nats.Msg) {
	topic := msg.Subject
	if f, ok := SubjectFuncMap[topic]; ok {
		err := f(msg)
		if err != nil {
			log.Println(err)
		}
		err = msg.Ack()
		if err != nil {
			log.Println(err)
		}
	} else {
		log.Println("no handler for topic")
	}

}

func handleMessage(msg *nats.Msg) error {
	topic := msg.Subject
	if f, ok := SubjectFuncMap[topic]; ok {
		return f(msg)
	}
	return nil
}

func testServiceConsume(msg *nats.Msg) error {
	ctx := context.Background()
	data := string(msg.Data)
	err := service.TestService.Test(&ctx, data)
	if err != nil {
		return err
	}
	fmt.Println("handle message success msg", msg.Header, msg.Reply)
	return nil
}
