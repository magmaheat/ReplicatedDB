.PHONY: clean build install

all: clean install

build: clean
	mkdir -p "../build"

proto:
	rm -rf ./config/proto/raft_service.pb.go
	rm -rf ./config/proto/raft_service_grpc.pb.go
	protoc --go_out=. --go-grpc_out=. config/proto/raft_service.proto

install: build
	go build -o ../build/server app/app_server/main.go
	go build -o ../build/client app/app_client/main.go

run_server: install
	cd ./../build && ./server

run_client:
	cd ./../build && ./client

clean:
	rm -rf ../build