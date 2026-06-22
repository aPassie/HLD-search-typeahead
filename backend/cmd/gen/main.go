// Command gen writes a synthetic `query,count` dataset for benchmarking when the
// real Wikipedia dump (scripts/fetch_dataset.sh) is too large to download. Counts
// follow a Zipf-like curve so short prefixes end up hot.
//
//	go run ./cmd/gen -n 150000 -out data/queries.csv
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
)

var vocab = strings.Fields(`
iphone ipad ipod macbook apple samsung galaxy pixel google android nokia sony lg
laptop desktop monitor keyboard mouse headphones earbuds speaker charger cable adapter
java javascript python golang rust kotlin swift typescript ruby php scala haskell
tutorial guide course book cheatsheet reference documentation example project template
nike adidas puma reebok shoes sneakers boots sandals jacket hoodie shirt jeans dress
coffee maker grinder espresso kettle blender toaster microwave oven fridge dishwasher
camera lens tripod drone gopro printer scanner router modem switch firewall
guitar piano violin drums bass synth microphone amplifier pedal headset
toyota honda tesla ford bmw audi mercedes nissan hyundai kia mazda
amazon netflix spotify youtube twitter reddit github stackoverflow wikipedia
chair desk table sofa lamp shelf cabinet mirror curtain pillow blanket
vitamin protein creatine omega collagen probiotic supplement powder capsule
backpack wallet watch sunglasses umbrella bottle mug flask thermos lunchbox
pizza burger sushi pasta salad sandwich tacos noodles curry steak dessert
flight hotel ticket booking vacation resort beach mountain city tour cruise
phone case screen protector cover stand mount holder grip wireless fast
best cheap top review price deal sale discount near online buy used new
`)

func main() {
	n := flag.Int("n", 150000, "number of distinct queries")
	out := flag.String("out", "data/queries.csv", "output CSV path")
	seed := flag.Int64("seed", 42, "rng seed")
	flag.Parse()

	r := rand.New(rand.NewSource(*seed))
	set := make(map[string]struct{}, *n)
	queries := make([]string, 0, *n)

	add := func(q string) {
		if _, ok := set[q]; !ok {
			set[q] = struct{}{}
			queries = append(queries, q)
		}
	}
	for _, w := range vocab { // single words first
		add(w)
	}
	for len(queries) < *n { // then 2- and 3-word combinations
		k := 2
		if r.Intn(3) == 0 {
			k = 3
		}
		parts := make([]string, k)
		for i := range parts {
			parts[i] = vocab[r.Intn(len(vocab))]
		}
		add(strings.Join(parts, " "))
	}
	queries = queries[:*n]
	r.Shuffle(len(queries), func(i, j int) { queries[i], queries[j] = queries[j], queries[i] })

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	defer bw.Flush()

	fmt.Fprintln(bw, "query,count")
	for i, q := range queries {
		count := int64(2_000_000/(i+1)) + int64(r.Intn(50)) + 1 // 1/rank, with a bit of jitter
		fmt.Fprintf(bw, "%s,%d\n", q, count)
	}
	fmt.Printf("wrote %d queries to %s\n", len(queries), *out)
}
