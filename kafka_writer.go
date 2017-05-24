package main

import (
	"context"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/sirupsen/logrus"
)

type kafkaWriter struct {
	producer *kafka.Producer
	topic    string
}

func newKafkaWriter(ctx context.Context, topic string, cfg kafka.ConfigMap) (*kafkaWriter, error) {
	p, err := kafka.NewProducer(&cfg)
	if err != nil {
		return nil, err
	}
	kw := &kafkaWriter{
		producer: p,
		topic:    topic,
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

	// set delivery channel `nil` because we will read from Events channel
	if err := kw.producer.Produce(msg, nil); err != nil {
		return 0, err
	}
	return len(value), nil
}

func (kw *kafkaWriter) handleResponse(ctx context.Context) {
	for {
		select {
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
