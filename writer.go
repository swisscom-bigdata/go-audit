package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/sirupsen/logrus"
)

type AuditWriter struct {
	e        *json.Encoder
	w        io.Writer
	attempts int
}

func NewAuditWriter(w io.Writer, attempts int) *AuditWriter {
	return &AuditWriter{
		e:        json.NewEncoder(w),
		w:        w,
		attempts: attempts,
	}
}

func (a *AuditWriter) Write(msg *AuditMessageGroup) (err error) {
	sentLogsTotal.WithLabelValues(hostname).Inc()
	for i := 0; i < a.attempts; i++ {
		err = a.e.Encode(msg)
		if err == nil {
			break
		}

		if i != a.attempts {
			// We have to reset the encoder because write errors are kept internally and can not be retried
			a.e = json.NewEncoder(a.w)
			logrus.WithError(err).Error("failed to write message, retrying in 1 second")
			time.Sleep(time.Second * 1)
		}
	}

	if err != nil {
		sentErrorsTotal.WithLabelValues(hostname).Inc()
		return err
	}
	return nil
}
