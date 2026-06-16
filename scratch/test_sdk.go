package main

import (
	"context"
	"github.com/cloudflare/cloudflare-go"
)

func main() {
	var api cloudflare.API
	// This will test if the method compiles
	_, _, _ = api.Memberships(context.Background(), cloudflare.MembershipListParams{})
}
