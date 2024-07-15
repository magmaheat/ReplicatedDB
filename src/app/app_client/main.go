package main

import (
	cl "Go_Team01/src/client"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	logFile := pathLogFile("client.log")
	defer logFile.Close()

	address := parseAddress()

	client, err := cl.NewClient(address)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if err = client.Initialize(); err != nil {
		log.Fatal(err)
	}

	client.Run()
}

func pathLogFile(path string) *os.File {
	logFile, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}

	log.SetOutput(logFile)
	return logFile
}

func parseAddress() string {
	host := flag.String("H", "127.0.0.1", "host for address")
	port := flag.String("P", "7865", "port for address")
	flag.Parse()

	address := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Println("address:", address)

	return address
}
