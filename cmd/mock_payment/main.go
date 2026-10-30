package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Standalone local simulator of a signed provider webhook. Not an HTTP service.
func main() {
	id := flag.Int64("id", 0, "payment ID returned by POST /finance/topups")
	amount := flag.String("amount", "", "exact amount with two decimals, e.g. 20.00")
	status := flag.String("status", "succeeded", "succeeded or failed")
	baseURL := flag.String("url", "http://127.0.0.1:5555", "backend address")
	flag.Parse()
	secret := os.Getenv("PAYMENT_SECRET")
	if len(secret) < 32 || *id < 1 || *amount == "" || (*status != "succeeded" && *status != "failed") {
		fmt.Fprintln(os.Stderr, "PAYMENT_SECRET and -id, -amount, -status are required")
		os.Exit(1)
	}
	event := map[string]any{
		"event_id":            fmt.Sprintf("mock_event_%d_%d", *id, time.Now().UnixNano()),
		"provider_payment_id": fmt.Sprintf("mock_%d", *id),
		"status":              *status, "amount_byn": *amount, "currency": "BYN",
	}
	body, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req, err := http.NewRequest(http.MethodPost, *baseURL+"/api/v1/integrations/payment-webhook", bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	content, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	fmt.Printf("Status: %d\n%s\n", res.StatusCode, string(content))
	if res.StatusCode != 204 {
		os.Exit(1)
	}
}
