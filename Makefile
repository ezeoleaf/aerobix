APP_NAME := aerobix

.PHONY: run build test fmt lint tidy clean

run:
	go run .

build:
	go build -o $(APP_NAME) .

test:
	go test ./...

fmt:
	gofmt -w main.go domain/*.go physics/*.go provider/*.go provider/mock/*.go provider/strava/*.go ui/*.go

lint:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -f $(APP_NAME)
