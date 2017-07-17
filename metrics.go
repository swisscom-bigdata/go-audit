package main

import "github.com/prometheus/client_golang/prometheus"

var (
	sentLogsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "goaudit",
			Name:      "sent_logs_total",
			Help:      "The amount of logs that were sent to Kafka.",
		}, []string{"host"},
	)

	inFlightLogs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "goaudit",
			Name:      "inflight_logs_total",
			Help:      "The amount of logs in flight in librdkafka queue",
		}, []string{"host"},
	)

	sentErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "goaudit",
			Name:      "sent_error_total",
			Help:      "The amount of errors while sending logs to Kafka.",
		}, []string{"host"},
	)

	sentLatencyNanoseconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "goaudit",
			Name:      "sent_latency_nanoseconds",
			Help:      "The latecncy to send log to Kafka.",
		}, []string{"host"},
	)
)

func init() {
	prometheus.MustRegister(sentLogsTotal)
	prometheus.MustRegister(inFlightLogs)
	prometheus.MustRegister(sentErrorsTotal)
	prometheus.MustRegister(sentLatencyNanoseconds)
}
