package scratch

import (
	"fmt"
	"github.com/yinheli/qqwry"
)

func runTestQqwry() {
	q := qqwry.NewQQwry("data/geo/qqwry.dat")
	q.Find("1.1.1.1")
	fmt.Printf("IP: %s | Country: %s | City: %s\n", q.Ip, q.Country, q.City)
}
