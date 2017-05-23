package main

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/sirupsen/logrus"
)

type kafkaWriter struct {
	producer       *kafka.Producer
	topic          string
	responseEvents chan kafka.Event
}

func newKafkaWriter(topic string, cfg kafka.ConfigMap) (*kafkaWriter, error) {
	p, err := kafka.NewProducer(&cfg)
	if err != nil {
		return nil, err
	}
	return &kafkaWriter{
		producer:       p,
		topic:          topic,
		responseEvents: make(chan kafka.Event),
	}, nil
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

func (kw *kafkaWriter) handleResponse() {
	for evt := range kw.responseEvents {
		if msg, ok := evt.(*kafka.Message); ok && msg.TopicPartition.Error != nil {
			logrus.WithError(msg.TopicPartition.Error).Error("failed to producer message")
		}
	}
}
