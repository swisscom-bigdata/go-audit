package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/karrick/goavro"
)

// Defines pissible encoders.
const (
	JSONEncoderType = "json"
	AvroEncoderType = "avro"
)

// EncoderConfig defines configuration for Encoder.
type EncoderConfig struct {
	Type              string `yaml:"type"`
	SchemaFile        string `yaml:"schema_file"`
	SchemaRegistryURL string `yaml:"schema_registry_url"`
	Topic             string `yaml:"topic"`
}

// NewEncoder creates new Kafka Encoder.
func NewEncoder(cfg EncoderConfig) (Encoder, error) {
	switch cfg.Type {
	case JSONEncoderType:
		return &jsonEncoder{}, nil
	case AvroEncoderType:
		schema, id, err := avroSchema(cfg)
		if err != nil {
			return nil, err
		}
		codec, err := goavro.NewCodec(schema)
		if err != nil {
			return nil, fmt.Errorf("failed to create Avro codec: %v", err)
		}
		return &avroEncoder{
			codec:    codec,
			schemaID: id,
		}, nil
	}

	return nil, fmt.Errorf("encoder is not supported: %s", cfg.Type)
}

type avroEncoder struct {
	codec    *goavro.Codec
	schemaID int
}

func (a *avroEncoder) Encode(data []byte) ([]byte, []byte, error) {
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, nil, err
	}
	value, err := a.codec.BinaryFromNative(nil, m)
	if err != nil {
		return nil, nil, err
	}

	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.BigEndian, int8(0))
	binary.Write(buf, binary.BigEndian, int32(a.schemaID))
	binary.Write(buf, binary.BigEndian, value)

	return []byte(m["hostname"].(string)), buf.Bytes(), nil
}

type jsonEncoder struct{}

func (*jsonEncoder) Encode(data []byte) ([]byte, []byte, error) {
	var amg AuditMessageGroup
	if err := json.Unmarshal(data, &amg); err != nil {
		return nil, nil, err
	}
	return []byte(amg.Hostname), data, nil
}

func avroSchema(cfg EncoderConfig) (string, int, error) {
	data, err := ioutil.ReadFile(cfg.SchemaFile)
	if err != nil {
		return "", -1, fmt.Errorf("failed to read Avro schema: %v", err)
	}
	httpClient := http.Client{
		Transport: http.DefaultTransport,
		Timeout:   1 * time.Minute,
	}

	var schema struct {
		Schema string `json:"schema"`
	}
	schema.Schema = string(data)

	body, err := json.Marshal(schema)
	if err != nil {
		return "", -1, err
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/subjects/%s-value/versions", cfg.SchemaRegistryURL, cfg.Topic),
		bytes.NewBuffer(body),
	)
	req.Header.Add("Content-Type", "application/vnd.schemaregistry.v1+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", -1, fmt.Errorf("failed to get schema ID: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var errorResp struct {
			ErrorCode int    `json:"error_code"`
			Message   string `json:"message"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			return "", -1, fmt.Errorf("%d error: %s", errorResp.ErrorCode, errorResp.Message)
		}
		return "", -1, fmt.Errorf("%d error: %s", resp.StatusCode, resp.Status)
	}

	var response struct {
		ID int `json:"id"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", -1, fmt.Errorf("malformed response: %v", err)
	}
	return string(data), response.ID, nil
}
