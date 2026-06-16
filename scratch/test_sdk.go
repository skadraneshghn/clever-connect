package main

import (
	"fmt"
	"reflect"
	"github.com/cloudflare/cloudflare-go"
)

func main() {
	printFields(cloudflare.PermissionGroup{})
}

func printFields(x interface{}) {
	t := reflect.TypeOf(x)
	fmt.Printf("\n--- Struct: %s ---\n", t.Name())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Printf("  %s %s\n", f.Name, f.Type)
	}
}
