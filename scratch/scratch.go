package scratch
import "fmt"
func runScratch() {
    limit := 0
    total := 100
    offset := 0
    all := make([]int, 100)
    for i := range all { all[i] = i }

    // logic from ListClientConfigs
    if limit > 0 {
		if offset >= total {
			fmt.Println("Empty", total)
            return
		}
		end := offset + limit
		if end > total {
			end = total
		}
		fmt.Println("Paginated", len(all[offset:end]), total)
        return
	}
	fmt.Println("All", len(all), total)
}
