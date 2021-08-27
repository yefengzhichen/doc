package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	handler := http.NewServeMux()
	handler.HandleFunc("/client/api/v1/users", controller)

	log.Fatal(http.ListenAndServe(":8090", handler))
}

func controller(w http.ResponseWriter, _ *http.Request) {
	time.Sleep(2 * time.Second)
	str := "test context"
	body := []byte(str)
	_, _ = w.Write(body)
}
