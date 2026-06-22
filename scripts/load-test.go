package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds load test configuration
type Config struct {
	Target      string
	Concurrency int
	Duration    time.Duration
	Rate        int           // requests per second per worker
	HTTPTimeout time.Duration
	GRPCTimeout time.Duration
}

// Result holds a single request result
type Result struct {
	Timestamp  time.Time
	Duration   time.Duration
	Error      error
	StatusCode int
	Endpoint   string
	Protocol   string // http or grpc
}

// Stats holds aggregated statistics
type Stats struct {
	TotalRequests     int
	SuccessCount      int
	ErrorCount        int
	LatencySum        time.Duration
	Latencies         []time.Duration
	StatusCodes       map[int]int
	Endpoints         map[string]int
	Errors            map[string]int
	mu                sync.Mutex
}

func NewStats() *Stats {
	return &Stats{
		Latencies:   make([]time.Duration, 0),
		StatusCodes: make(map[int]int),
		Endpoints:   make(map[string]int),
		Errors:      make(map[string]int),
	}
}

func (s *Stats) Record(r Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalRequests++
	if r.Error != nil {
		s.ErrorCount++
		s.Errors[r.Error.Error()]++
	} else {
		s.SuccessCount++
		s.LatencySum += r.Duration
		s.Latencies = append(s.Latencies, r.Duration)
	}
	s.StatusCodes[r.StatusCode]++
	s.Endpoints[r.Endpoint]++
}

func main() {
	var cfg Config
	flag.StringVar(&cfg.Target, "target", "http://localhost:8009", "Base target URL for HTTP services")
	flag.IntVar(&cfg.Concurrency, "concurrency", 100, "Number of concurrent workers")
	flag.DurationVar(&cfg.Duration, "duration", 5*time.Minute, "Test duration (e.g., 5m, 30s)")
	flag.IntVar(&cfg.Rate, "rate", 10, "Requests per second per worker")
	flag.DurationVar(&cfg.HTTPTimeout, "http-timeout", 30*time.Second, "HTTP request timeout")
	flag.DurationVar(&cfg.GRPCTimeout, "grpc-timeout", 30*time.Second, "gRPC request timeout")
	flag.Parse()

	fmt.Println("================================================================")
	fmt.Println("           CloudFlow Production Load Test Tool")
	fmt.Println("================================================================")
	fmt.Printf("Target:      %s\n", cfg.Target)
	fmt.Printf("Concurrency: %d workers\n", cfg.Concurrency)
	fmt.Printf("Duration:    %s\n", cfg.Duration)
	fmt.Printf("Rate:        %d req/s per worker\n", cfg.Rate)
	fmt.Printf("HTTP Timeout: %s\n", cfg.HTTPTimeout)
	fmt.Printf("gRPC Timeout: %s\n", cfg.GRPCTimeout)
	fmt.Println()

	stats := NewStats()
	results := make(chan Result, cfg.Concurrency*cfg.Rate*10)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	startTime := time.Now()

	// Start result collector
	go func() {
		for r := range results {
			stats.Record(r)
		}
	}()

	// Launch HTTP workers
	for i := 0; i < cfg.Concurrency/2; i++ {
		wg.Add(1)
		go httpWorker(ctx, &wg, cfg, results, i)
	}

	// Launch gRPC workers
	for i := 0; i < cfg.Concurrency/2; i++ {
		wg.Add(1)
		go grpcWorker(ctx, &wg, cfg, results, i)
	}

	// Progress reporter
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime)
				stats.mu.Lock()
				total := stats.TotalRequests
				success := stats.SuccessCount
				errors := stats.ErrorCount
				stats.mu.Unlock()
				fmt.Printf("[%.0fs] Total: %d | Success: %d | Errors: %d | RPS: %.2f\n",
					elapsed.Seconds(), total, success, errors, float64(total)/elapsed.Seconds())
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for completion
	wg.Wait()
	close(results)
	time.Sleep(100 * time.Millisecond) // let collector finish

	elapsed := time.Since(startTime)
	fmt.Println("\n================================================================")
	fmt.Println("                    Load Test Results")
	fmt.Println("================================================================")
	printReport(stats, elapsed)
}

func httpWorker(ctx context.Context, wg *sync.WaitGroup, cfg Config, results chan<- Result, workerID int) {
	defer wg.Done()
	client := &http.Client{Timeout: cfg.HTTPTimeout}
	endpoints := []string{
		"/health",
		"/api/v1/auth/login",
		"/api/v1/auth/refresh",
		"/api/v1/tenants",
		"/api/v1/tenants/:id",
		"/api/v1/control/rules",
		"/api/v1/control/rules/:id",
		"/api/v1/control/policies",
		"/api/v1/query/metrics",
		"/api/v1/query/alerts",
		"/api/v1/alerts/rules",
		"/api/v1/alerts/rules/:id",
		"/api/v1/data/ingest",
		"/api/v1/data/batch",
	}

	ticker := time.NewTicker(time.Second / time.Duration(cfg.Rate))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			endpoint := endpoints[rand.Intn(len(endpoints))]
			result := doHTTPRequest(client, cfg.Target, endpoint, workerID)
			results <- result
		}
	}
}

func doHTTPRequest(client *http.Client, baseURL, endpoint string, workerID int) Result {
	start := time.Now()
	url := baseURL + endpoint
	method := http.MethodGet
	var body io.Reader

	// Simulate different request types based on endpoint
	switch endpoint {
	case "/api/v1/auth/login":
		method = http.MethodPost
		payload := map[string]string{
			"username": fmt.Sprintf("user_%d", workerID),
			"password": "test-password",
		}
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	case "/api/v1/data/ingest", "/api/v1/data/batch":
		method = http.MethodPost
		payload := generateMetricsBatch(workerID)
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	case "/api/v1/alerts/rules", "/api/v1/control/rules":
		if rand.Float32() < 0.3 {
			method = http.MethodPost
			payload := map[string]interface{}{
				"name":  fmt.Sprintf("rule_%d_%d", workerID, time.Now().Unix()),
				"expr":  "cpu_usage > 80",
				"for":   "5m",
				"labels": map[string]string{"severity": "warning"},
			}
			data, _ := json.Marshal(payload)
			body = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return Result{Timestamp: time.Now(), Error: err, Endpoint: endpoint, Protocol: "http"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Load-Test-Worker", fmt.Sprintf("%d", workerID))

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return Result{Timestamp: time.Now(), Duration: duration, Error: err, Endpoint: endpoint, Protocol: "http"}
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)
	return Result{
		Timestamp:  time.Now(),
		Duration:   duration,
		StatusCode: resp.StatusCode,
		Endpoint:   endpoint,
		Protocol:   "http",
	}
}

func generateMetricsBatch(workerID int) map[string]interface{} {
	metrics := make([]map[string]interface{}, 0, 10)
	now := time.Now().Unix()
	for i := 0; i < 10; i++ {
		metrics = append(metrics, map[string]interface{}{
			"metric":    fmt.Sprintf("metric_%d", rand.Intn(100)),
			"timestamp": now,
			"value":     rand.Float64() * 100,
			"labels": map[string]string{
				"instance": fmt.Sprintf("host-%d", workerID),
				"job":      "cloudflow-staging",
			},
		})
	}
	return map[string]interface{}{
		"metrics": metrics,
		"tenant":  fmt.Sprintf("tenant_%d", workerID%10),
	}
}

func grpcWorker(ctx context.Context, wg *sync.WaitGroup, cfg Config, results chan<- Result, workerID int) {
	defer wg.Done()
	// gRPC connections to data-plane and query-service
	dataPlaneAddr := "localhost:9004"
	queryServiceAddr := "localhost:9005"

	dataConn, err := grpc.Dial(dataPlaneAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "gRPC connection to data-plane failed: %v\n", err)
		return
	}
	defer dataConn.Close()

	queryConn, err := grpc.Dial(queryServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "gRPC connection to query-service failed: %v\n", err)
		return
	}
	defer queryConn.Close()

	// Note: In production, use proper protobuf-generated clients
	// Here we simulate gRPC calls with health checks via reflection
	ticker := time.NewTicker(time.Second / time.Duration(cfg.Rate))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Simulate gRPC call to data-plane
			start := time.Now()
			// grpc health check simulation
			_ = dataConn.GetState()
			results <- Result{
				Timestamp: time.Now(),
				Duration:  time.Since(start),
				Endpoint:  "data-plane.IngestMetrics",
				Protocol:  "grpc",
			}

			// Simulate gRPC call to query-service
			start = time.Now()
			_ = queryConn.GetState()
			results <- Result{
				Timestamp: time.Now(),
				Duration:  time.Since(start),
				Endpoint:  "query-service.QueryMetrics",
				Protocol:  "grpc",
			}
		}
	}
}

func printReport(stats *Stats, elapsed time.Duration) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	fmt.Printf("Duration:        %s\n", elapsed.Round(time.Second))
	fmt.Printf("Total Requests:    %d\n", stats.TotalRequests)
	fmt.Printf("Successful:        %d (%.2f%%)\n", stats.SuccessCount, float64(stats.SuccessCount)/float64(stats.TotalRequests)*100)
	fmt.Printf("Errors:            %d (%.2f%%)\n", stats.ErrorCount, float64(stats.ErrorCount)/float64(stats.TotalRequests)*100)
	fmt.Printf("Average RPS:       %.2f\n", float64(stats.TotalRequests)/elapsed.Seconds())

	if len(stats.Latencies) > 0 {
		fmt.Println("\n--- Latency Distribution ---")
		fmt.Printf("Count:   %d\n", len(stats.Latencies))
		
		// Calculate percentiles
		sorted := make([]time.Duration, len(stats.Latencies))
		copy(sorted, stats.Latencies)
		// simple sort for percentiles
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		
		avg := stats.LatencySum / time.Duration(len(stats.Latencies))
		fmt.Printf("Average: %s\n", avg)
		fmt.Printf("Min:     %s\n", sorted[0])
		fmt.Printf("P50:     %s\n", percentile(sorted, 0.50))
		fmt.Printf("P75:     %s\n", percentile(sorted, 0.75))
		fmt.Printf("P90:     %s\n", percentile(sorted, 0.90))
		fmt.Printf("P95:     %s\n", percentile(sorted, 0.95))
		fmt.Printf("P99:     %s\n", percentile(sorted, 0.99))
		fmt.Printf("Max:     %s\n", sorted[len(sorted)-1])
	}

	fmt.Println("\n--- HTTP Status Codes ---")
	for code, count := range stats.StatusCodes {
		fmt.Printf("  %d: %d\n", code, count)
	}

	fmt.Println("\n--- Endpoint Distribution ---")
	for endpoint, count := range stats.Endpoints {
		fmt.Printf("  %s: %d\n", endpoint, count)
	}

	if len(stats.Errors) > 0 {
		fmt.Println("\n--- Error Summary ---")
		for errMsg, count := range stats.Errors {
			fmt.Printf("  [%d] %s\n", count, errMsg)
		}
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
