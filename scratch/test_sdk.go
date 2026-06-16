package main

import (
	"fmt"
	"reflect"
	"github.com/cloudflare/cloudflare-go"
)

func main() {
	printFields(cloudflare.D1Database{})
	printFields(cloudflare.CreateD1DatabaseParams{})
	printFields(cloudflare.ListD1DatabasesParams{})
}

func printFields(x interface{}) {
	t := reflect.TypeOf(x)
	fmt.Printf("\n--- Struct: %s ---\n", t.String())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Printf("  %s %s\n", f.Name, f.Type)
	}
}
