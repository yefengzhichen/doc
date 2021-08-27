package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	req, err := http.NewRequest(http.MethodGet, "http://localhost:8000/info", nil)
	client := http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)
}
