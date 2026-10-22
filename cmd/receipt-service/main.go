package main

import (
	"log"
	"net/http"
	"os"

	"example.com/release-receipt-service/internal/receipt"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}

	mux := http.NewServeMux()
	mux.Handle("POST /build-events", receipt.NewHandler(receipt.NewClient(key)))
	log.Printf("receipt service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}
