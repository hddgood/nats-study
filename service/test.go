package service

import (
	"context"
	"log"
)

var TestService = new(testService)

type testService struct {
}

func (t *testService) Test(ctx *context.Context, data string) error {

	log.Println("testService Consume, data: ", data)

	return nil
}
