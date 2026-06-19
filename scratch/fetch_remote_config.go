package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
)

func main() {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJyb2xlIjoiYWRtaW4iLCJ1c2VybmFtZSI6InNhbG1hbiJ9.0ojXc0fp39Ia7NjBQMnzGXy8T_vstuU3kqaFcxpEIdE"
	url := "https://ondata.ir/api/ehco/config"

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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response Status: %s (%d)\n", resp.Status, resp.StatusCode)
	fmt.Printf("Body: %s\n", string(body))
}
