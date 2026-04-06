// Binary loadtest drives read/write traffic and records latency + stale-read stats.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type OpKind string

const (
	OpWrite OpKind = "write"
	OpRead  OpKind = "read"
)

type sample struct {
	Kind              OpKind  `json:"kind"`
	LatencyMS         float64 `json:"latency_ms"`
	Key               string  `json:"key"`
	Stale             bool    `json:"stale"`
	SinceWriteSameKey float64 `json:"since_write_same_key_ms,omitempty"`
	Version           int64   `json:"version,omitempty"`
	ExpectedVersion   int64   `json:"expected_version,omitempty"`
}

type kvEntry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}

type client struct {
	http       *http.Client
	mode       string
	leader     string
	readURLs   []string
	keyPrefix  string
	keyPool    int
	writeRatio float64
	// last known version per key (from our writes / strongest observed read)
	expVer map[string]int64
	lastW  map[string]time.Time
	mu     sync.RWMutex
	samples []sample
	sMu    sync.Mutex
	rng    *rand.Rand
}

func (c *client) addSample(s sample) {
	c.sMu.Lock()
	c.samples = append(c.samples, s)
	c.sMu.Unlock()
}

func (c *client) setExpected(key string, ver int64) {
	c.mu.Lock()
	c.expVer[key] = ver
	c.lastW[key] = time.Now()
	c.mu.Unlock()
}

func (c *client) getExpected(key string) (int64, time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ver, ok := c.expVer[key]
	t := c.lastW[key]
	return ver, t, ok
}

func (c *client) observeRead(key string, ver int64) {
	c.mu.Lock()
	if ver > c.expVer[key] {
		c.expVer[key] = ver
	}
	c.mu.Unlock()
}

func (c *client) doWrite(target, key, value string) (kvEntry, float64, int, error) {
	body, _ := json.Marshal(map[string]string{"key": key, "value": value})
	start := time.Now()
	req, err := http.NewRequest(http.MethodPost, target+"/kv/set", bytes.NewReader(body))
	if err != nil {
		return kvEntry{}, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	ms := float64(time.Since(start).Milliseconds())
	if err != nil {
		return kvEntry{}, ms, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return kvEntry{}, ms, resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	var meta struct {
		Version int64 `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	return kvEntry{Value: value, Version: meta.Version}, ms, resp.StatusCode, nil
}

func (c *client) doRead(target, key string) (kvEntry, float64, int, error) {
	start := time.Now()
	resp, err := c.http.Get(target + "/kv/get?key=" + url.QueryEscape(key))
	ms := float64(time.Since(start).Milliseconds())
	if err != nil {
		return kvEntry{}, ms, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return kvEntry{}, ms, resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return kvEntry{}, ms, resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	var e kvEntry
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return kvEntry{}, ms, resp.StatusCode, err
	}
	return e, ms, resp.StatusCode, nil
}

func (c *client) worker(stop <-chan struct{}, wg *sync.WaitGroup, staleCount *atomic.Int64) {
	defer wg.Done()
	hotFrac := 0.85
	for {
		select {
		case <-stop:
			return
		default:
		}
		isWrite := c.rng.Float64() < c.writeRatio
		keyNum := c.rng.Intn(c.keyPool)
		key := fmt.Sprintf("%s%03d", c.keyPrefix, keyNum)

		if !isWrite {
			if c.rng.Float64() < hotFrac {
				c.mu.RLock()
				var candidates []int
				for k := range c.lastW {
					if strings.HasPrefix(k, c.keyPrefix) {
						n, err := strconv.Atoi(strings.TrimPrefix(k, c.keyPrefix))
						if err == nil {
							candidates = append(candidates, n)
						}
					}
				}
				c.mu.RUnlock()
				if len(candidates) > 0 {
					keyNum = candidates[c.rng.Intn(len(candidates))]
					key = fmt.Sprintf("%s%03d", c.keyPrefix, keyNum)
				}
			}
		}

		if isWrite {
			val := fmt.Sprintf("v-%d", time.Now().UnixNano())
			target := c.leader
			if c.mode == "leaderless" {
				target = c.readURLs[c.rng.Intn(len(c.readURLs))]
			}
			ent, ms, _, err := c.doWrite(target, key, val)
			if err != nil {
				continue
			}
			c.setExpected(key, ent.Version)
			c.addSample(sample{Kind: OpWrite, LatencyMS: ms, Key: key, Version: ent.Version, ExpectedVersion: ent.Version})
		} else {
			target := c.readURLs[c.rng.Intn(len(c.readURLs))]
			exp, tw, hasExp := c.getExpected(key)
			ent, ms, code, err := c.doRead(target, key)
			if err != nil || code == http.StatusNotFound {
				continue
			}
			c.observeRead(key, ent.Version)
			var since float64
			if !tw.IsZero() {
				since = float64(time.Since(tw).Milliseconds())
			}
			stale := hasExp && ent.Version < exp
			if stale {
				staleCount.Add(1)
			}
			c.addSample(sample{
				Kind:              OpRead,
				LatencyMS:         ms,
				Key:               key,
				Stale:             stale,
				SinceWriteSameKey: since,
				Version:           ent.Version,
				ExpectedVersion:   exp,
			})
		}
	}
}

func main() {
	var (
		mode        = flag.String("mode", getenv("MODE", "leader-follower"), "leader-follower | leaderless")
		leader      = flag.String("leader", getenv("LEADER_URL", ""), "base URL of leader (LF writes)")
		endpoints   = flag.String("endpoints", getenv("ENDPOINTS", ""), "comma URLs for reads (and leaderless writes); LF: all nodes")
		writeRatio  = flag.Float64("write-ratio", 0.5, "probability each op is a write")
		workers     = flag.Int("workers", 8, "concurrent workers")
		runFor      = flag.Duration("duration", 30*time.Second, "test duration")
		keyPool     = flag.Int("key-pool", 40, "number of keys (local-in-time locality)")
		keyPrefix   = flag.String("key-prefix", "k-", "key prefix")
		outPath     = flag.String("out", "loadtest_results.json", "JSON output path")
		profile     = flag.String("profile", getenv("QUORUM_PROFILE", ""), "optional label for LF profile")
	)
	flag.Parse()

	eps := parseCSV(*endpoints)
	if len(eps) == 0 {
		log.Fatal("need -endpoints or ENDPOINTS")
	}
	if *mode == "leader-follower" && *leader == "" {
		*leader = eps[0]
		log.Printf("defaulting leader to first endpoint: %s", *leader)
	}

	c := &client{
		http: &http.Client{Timeout: 120 * time.Second},
		mode: *mode,
		leader: strings.TrimRight(*leader, "/"),
		readURLs: eps,
		keyPrefix: *keyPrefix,
		keyPool: *keyPool,
		writeRatio: *writeRatio,
		expVer: make(map[string]int64),
		lastW: make(map[string]time.Time),
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var stale atomic.Int64
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go c.worker(stop, &wg, &stale)
	}
	time.Sleep(*runFor)
	close(stop)
	wg.Wait()

	c.sMu.Lock()
	samples := append([]sample(nil), c.samples...)
	c.sMu.Unlock()

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].LatencyMS < samples[j].LatencyMS
	})

	type summary struct {
		Mode            string  `json:"mode"`
		Profile         string  `json:"quorum_profile,omitempty"`
		WriteRatio      float64 `json:"write_ratio"`
		DurationSec     float64 `json:"duration_sec"`
		TotalSamples    int     `json:"total_samples"`
		ReadCount       int     `json:"reads"`
		WriteCount      int     `json:"writes"`
		StaleReads      int64   `json:"stale_reads"`
		Workers         int     `json:"workers"`
		KeyPool         int     `json:"key_pool"`
		Endpoints       int     `json:"endpoints"`
	}

	rc, wc := 0, 0
	for _, s := range samples {
		switch s.Kind {
		case OpRead:
			rc++
		case OpWrite:
			wc++
		}
	}

	out := struct {
		Summary summary  `json:"summary"`
		Samples []sample `json:"samples"`
	}{
		Summary: summary{
			Mode:         *mode,
			Profile:      *profile,
			WriteRatio:   *writeRatio,
			DurationSec:  runFor.Seconds(),
			TotalSamples: len(samples),
			ReadCount:    rc,
			WriteCount:   wc,
			StaleReads:   stale.Load(),
			Workers:      *workers,
			KeyPool:      *keyPool,
			Endpoints:    len(eps),
		},
		Samples: samples,
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, b, 0644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d ops, %d stale reads)", *outPath, len(samples), stale.Load())
}
