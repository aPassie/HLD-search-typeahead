// Command bench drives concurrent load at GET /suggest and reports client-side
// latency percentiles + throughput, then prints the server's /metrics. Prefixes
// are derived from the dataset so short, shared prefixes get hit most.
//
//	go run ./cmd/bench -addr http://localhost:8080 -c 50 -d 10s
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	addr := flag.String("addr", "http://localhost:8080", "server base URL")
	data := flag.String("data", "data/queries.csv", "dataset to derive prefixes from")
	conc := flag.Int("c", 50, "concurrent workers")
	dur := flag.Duration("d", 10*time.Second, "measurement duration")
	warm := flag.Duration("warmup", 2*time.Second, "warmup duration (also fills cache)")
	mode := flag.String("mode", "count", "suggest mode: count|recency")
	seed := flag.Int64("seed", 7, "rng seed")
	flag.Parse()

	prefixes := loadPrefixes(*data)
	if len(prefixes) == 0 {
		fmt.Fprintln(os.Stderr, "no prefixes loaded")
		os.Exit(1)
	}
	fmt.Printf("loaded %d prefix samples; %d workers, %s warmup + %s measure, mode=%s\n",
		len(prefixes), *conc, *warm, *dur, *mode)

	client := &http.Client{Timeout: 5 * time.Second}
	base := *addr + "/suggest?mode=" + *mode + "&q="

	fire := func(deadline time.Time, record bool) ([]time.Duration, int64, int64) {
		var wg sync.WaitGroup
		var reqs, errs atomic.Int64
		var mu sync.Mutex
		all := []time.Duration{}
		for wkr := 0; wkr < *conc; wkr++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				rr := rand.New(rand.NewSource(*seed + int64(id)))
				var local []time.Duration
				for time.Now().Before(deadline) {
					p := prefixes[rr.Intn(len(prefixes))]
					start := time.Now()
					resp, err := client.Get(base + url.QueryEscape(p))
					if err != nil {
						errs.Add(1)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					reqs.Add(1)
					if record {
						local = append(local, time.Since(start))
					}
				}
				if record && len(local) > 0 {
					mu.Lock()
					all = append(all, local...)
					mu.Unlock()
				}
			}(wkr)
		}
		wg.Wait()
		return all, reqs.Load(), errs.Load()
	}

	fire(time.Now().Add(*warm), false) // warm the cache; don't record these
	start := time.Now()
	durs, reqs, errs := fire(time.Now().Add(*dur), true)
	elapsed := time.Since(start)

	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	fmt.Printf("\nrequests: %d   errors: %d   wall: %s   throughput: %.0f req/s\n",
		reqs, errs, elapsed.Round(time.Millisecond), float64(reqs)/elapsed.Seconds())
	if len(durs) > 0 {
		fmt.Printf("client latency   p50: %s   p95: %s   p99: %s   max: %s\n",
			pct(durs, 50), pct(durs, 95), pct(durs, 99), durs[len(durs)-1])
	}

	if resp, err := client.Get(*addr + "/metrics"); err == nil {
		var m any
		json.NewDecoder(resp.Body).Decode(&m)
		resp.Body.Close()
		b, _ := json.MarshalIndent(m, "", "  ")
		fmt.Println("\nserver /metrics:")
		fmt.Println(string(b))
	}
}

func pct(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := p * len(sorted) / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i].Round(time.Microsecond)
}

// loadPrefixes emits the 1-4 character prefixes of each query, with repetition.
// Picking uniformly from the result then favours short prefixes shared by many
// queries, which is roughly how real typeahead traffic skews.
func loadPrefixes(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		comma := strings.LastIndexByte(line, ',')
		if comma <= 0 {
			continue
		}
		q := strings.TrimSpace(line[:comma])
		if q == "" || q == "query" {
			continue
		}
		runes := []rune(q)
		for n := 1; n <= 4 && n <= len(runes); n++ {
			out = append(out, string(runes[:n]))
		}
	}
	return out
}
