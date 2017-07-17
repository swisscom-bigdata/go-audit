package main

import (
	"context"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/sirupsen/logrus"
)

// Encoder encodes data from auditd to publish it in Kafka.
type Encoder interface {
	Encode(data []byte) (value []byte, err error)
}

// KafkaConfig defines configuration for Kafka Writer.
type KafkaConfig struct {
	Enabled  bool            `yaml:"enabled"`
	Attempts int             `yaml:"attempts"`
	Topic    string          `yaml:"topic"`
	Encoder  EncoderConfig   `yaml:"encoder"`
	Config   kafka.ConfigMap `yaml:"config"`
}

// KafkaWriter is an io.Writer that writes to the Kafka.
type KafkaWriter struct {
	producer *kafka.Producer
	topic    string
	enc      Encoder
}

// NewKafkaWriter creates new KafkaWrite.
func NewKafkaWriter(ctx context.Context, cfg KafkaConfig) (*KafkaWriter, error) {
	cfg.Encoder.Topic = cfg.Topic
	enc, err := NewEncoder(cfg.Encoder)
	if err != nil {
		return nil, err
	}

	p, err := kafka.NewProducer(&cfg.Config)
	if err != nil {
		return nil, err
	}
	kw := &KafkaWriter{
		producer: p,
		topic:    cfg.Topic,
		enc:      enc,
	}
	go kw.handleResponse(ctx)
	return kw, nil
}

// Write writes data to the Kafka, implements io.Writer.
func (kw *KafkaWriter) Write(value []byte) (int, error) {
	inFlightLogs.WithLabelValues(hostname).Inc()
	value, err := kw.enc.Encode(value)
	if err != nil {
		return 0, err
	}
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

func (kw *KafkaWriter) handleResponse(ctx context.Context) {
	for {
		select {
		case evt := <-kw.producer.Events():

			if msg, ok := evt.(*kafka.Message); ok {
				if msg.TopicPartition.Error != nil {
					logrus.WithError(msg.TopicPartition.Error).Error("failed to producer message")
					sentErrorsTotal.WithLabelValues(hostname).Inc()
				}

				inFlightLogs.WithLabelValues(hostname).Dec()
				sentLatencyNanoseconds.WithLabelValues(hostname).Observe(float64(time.Since(msg.Timestamp)))
			}
		case <-ctx.Done():
			kw.producer.Close()
			return
		}
	}
}
