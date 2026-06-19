package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
)

func check(token string) {
	url := fmt.Sprintf("https://ondata.ir/tunnel/%s%%3Fmux=false", token)
	fmt.Printf("Probing URL: %s\n", url)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response Status: %s (%d)\n", resp.Status, resp.StatusCode)
	for k, v := range resp.Header {
		fmt.Printf("  %s: %v\n", k, v)
	}
}

func main() {
	check("93a0ece5e1f869ea4730ce0296b9f6b0")
}
