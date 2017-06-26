GO_VERSION 			:= 1.8
GO_AUDIT_PACKAGE	:= github.com/slackhq/go-audit

vendor-sync:
	govendor sync

build: vendor-sync
build:
	go build

test: vendor-sync
test:
	go test -v

test-cov-html:
	go test -coverprofile=coverage.out
	go tool cover -html=coverage.out

bench:
	go test -bench=.

bench-cpu:
	go test -bench=. -benchtime=5s -cpuprofile=cpu.pprof
	go tool pprof go-audit.test cpu.pprof

bench-cpu-long:
	go test -bench=. -benchtime=60s -cpuprofile=cpu.pprof
	go tool pprof go-audit.test cpu.pprof

pull:
	docker pull golang:${GO_VERSION}

builder: pull
builder:
	docker build  -t golang-librdkafka -f Dockerfile.build .

docker-build: builder
docker-build:
	docker run --rm -v ${CURDIR}:/go/src/${GO_AUDIT_PACKAGE} -w /go/src/${GO_AUDIT_PACKAGE}  golang-librdkafka go build -v

docker-test: builder
docker-test:
	docker run --rm -v ${CURDIR}:/go/src/${GO_AUDIT_PACKAGE} -w /go/src/${GO_AUDIT_PACKAGE}  golang-librdkafka go test -v

.PHONY: test test-cov-html bench bench-cpu bench-cpu-long build
.DEFAULT_GOAL := build
