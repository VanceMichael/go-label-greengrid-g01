GO ?= go

.PHONY: build test race vet run docker-build
build:
	GOTOOLCHAIN=local $(GO) build ./...
test:
	GOTOOLCHAIN=local $(GO) test ./... -count=1
race:
	GOTOOLCHAIN=local $(GO) test -race ./... -count=1
vet:
	GOTOOLCHAIN=local $(GO) vet ./...
run:
	GOTOOLCHAIN=local $(GO) run ./cmd/server
docker-build:
	docker build -t greengrid:local .
