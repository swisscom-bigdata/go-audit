package main

import (
	"context"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/sirupsen/logrus"
)

type kafkaWriter struct {
	producer       *kafka.Producer
	topic          string
	responseEvents chan kafka.Event
}

func newKafkaWriter(ctx context.Context, topic string, cfg kafka.ConfigMap) (*kafkaWriter, error) {
	p, err := kafka.NewProducer(&cfg)
	if err != nil {
		return nil, err
	}
	kw := &kafkaWriter{
		producer:       p,
		topic:          topic,
		responseEvents: make(chan kafka.Event),
	}
	go kw.handleResponse(ctx)
	return kw, nil
}

func (kw *kafkaWriter) Write(value []byte) (int, error) {
	msg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &kw.topic,
			Partition: kafka.PartitionAny,
		},
		Value: value,
	}

	if err := kw.producer.Produce(msg, kw.responseEvents); err != nil {
		return 0, err
	}
	return len(value), nil
}

func (kw *kafkaWriter) handleResponse(ctx context.Context) {
	for {
		select {
		case evt := <-kw.responseEvents:
			if msg, ok := evt.(*kafka.Message); ok && msg.TopicPartition.Error != nil {
				logrus.WithError(msg.TopicPartition.Error).Error("failed to producer message")
			}
		case evt := <-kw.producer.Events():
			if msg, ok := evt.(*kafka.Message); ok && msg.TopicPartition.Error != nil {
				logrus.WithError(msg.TopicPartition.Error).Error("failed to producer message")
			}
		case <-ctx.Done():
			kw.producer.Close()
			return
		}
	}
}
