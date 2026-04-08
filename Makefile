.PHONY: build run test db-up db-down docker-up docker-down clean

build:
	go build -o server ./cmd/server

run: build
	./server

test:
	go test ./...

db-up:
	docker-compose up -d postgres

db-down:
	docker-compose down

docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down -v

clean:
	rm -f server
