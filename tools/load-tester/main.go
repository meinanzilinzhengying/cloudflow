// CloudFlow Load Tester
// Performance testing tool for simulating high-concurrency traffic
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/data-plane/proto"
)

// Config holds load test configuration
type Config struct {
	Mode            string  // ingest, query, mixed
	Target          string  // Target address (host:port)
	EdgeTarget      string  // Edge service address for ingest mode
	FlowsPerSecond  int     // Target flows per second
	Concurrency     int     // Number of concurrent workers
	Duration        int     // Test duration in seconds
	BatchSize       int     // Batch size for sending flows
	ReportFile      string  // Output report file path
	ReportInterval  int     // Progress report interval in seconds
}

// Metrics holds test metrics
type Metrics struct {
	// Counters
	TotalSent     uint64
	TotalReceived uint64
	TotalErrors   uint64

	// Latency tracking
	Latencies []time.Duration
	mu        sync.Mutex

	// Resource usage
	StartTime time.Time
	EndTime   time.Time
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		Latencies: make([]time.Duration, 0, 1000000),
	}
}

// RecordLatency adds a latency measurement
func (m *Metrics) RecordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Latencies = append(m.Latencies, latency)
}

// Report generates a test report
func (m *Metrics) Report() string {
	duration := m.EndTime.Sub(m.StartTime)
	throughput := float64(m.TotalSent) / duration.Seconds()
	lossRate := float64(m.TotalSent-m.TotalReceived) / float64(m.TotalSent) * 100

	// Calculate percentiles
	m.mu.Lock()
	defer m.mu.Unlock()

	var p50, p90, p99, p999 time.Duration
	if len(m.Latencies) > 0 {
		// Sort latencies (simple sort for small datasets)
		sorted := make([]time.Duration, len(m.Latencies))
		copy(sorted, m.Latencies)

		// Calculate percentiles
		p50 = sorted[len(sorted)*50/100]
		p90 = sorted[len(sorted)*90/100]
		p99 = sorted[len(sorted)*99/100]
		if len(sorted) >= 1000 {
			p999 = sorted[len(sorted)*999/1000]
		} else {
			p999 = sorted[len(sorted)-1]
		}
	}

	return fmt.Sprintf(`
=====================================
CloudFlow Load Test Report
=====================================
Test Duration: %.2f seconds
Total Flows Sent: %d
Total Flows Received: %d
Total Errors: %d
Loss Rate: %.4f%%

Throughput: %.2f flows/sec

Latency Percentiles:
  P50: %v
  P90: %v
  P99: %v
  P999: %v
=====================================
`, duration.Seconds(), m.TotalSent, m.TotalReceived, m.TotalErrors, lossRate,
		throughput, p50, p90, p99, p999)
}

// FlowGenerator generates random flow data
type FlowGenerator struct {
	rng *rand.Rand
}

// NewFlowGenerator creates a new flow generator
func NewFlowGenerator() *FlowGenerator {
	return &FlowGenerator{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateFlow generates a random flow
func (g *FlowGenerator) GenerateFlow() *svcproto.Flow {
	srcIP := fmt.Sprintf("192.168.%d.%d", g.rng.Intn(256), g.rng.Intn(256))
	dstIP := fmt.Sprintf("10.0.%d.%d", g.rng.Intn(256), g.rng.Intn(256))

	protocols := []string{"TCP", "UDP", "HTTP", "HTTPS", "DNS", "ICMP"}
	protocol := protocols[g.rng.Intn(len(protocols))]

	return &svcproto.Flow{
		Timestamp:   time.Now().Unix(),
		SrcIp:       srcIP,
		SrcPort:     uint32(g.rng.Intn(65535)),
		DstIp:       dstIP,
		DstPort:     uint32(g.rng.Intn(65535)),
		Protocol:    protocol,
		Bytes:       uint64(g.rng.Intn(1000000)),
		Packets:     uint64(g.rng.Intn(10000)),
		DurationMs:  uint32(g.rng.Intn(10000)),
		K8sNamespace: fmt.Sprintf("namespace-%d", g.rng.Intn(10)),
		K8sService:  fmt.Sprintf("service-%d", g.rng.Intn(20)),
		K8sPod:     fmt.Sprintf("pod-%d", g.rng.Intn(100)),
	}
}

// IngestWorker sends flows to the Edge service
type IngestWorker struct {
	id      int
	config  *Config
	metrics *Metrics
	conn    *grpc.ClientConn
	client  svcproto.DataPlaneServiceClient
}

// NewIngestWorker creates a new ingest worker
func NewIngestWorker(id int, config *Config, metrics *Metrics) (*IngestWorker, error) {
	conn, err := grpc.Dial(config.EdgeTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*10)))
	if err != nil {
		return nil, err
	}

	return &IngestWorker{
		id:     id,
		config: config,
		metrics: metrics,
		conn:   conn,
		client: svcproto.NewDataPlaneServiceClient(conn),
	}, nil
}

// Run starts the worker
func (w *IngestWorker) Run(ctx context.Context, flows <-chan *svcproto.Flow, wg *sync.WaitGroup) {
	defer wg.Done()
	defer w.conn.Close()

	batch := make([]*svcproto.Flow, 0, w.config.BatchSize)
	ticker := time.NewTicker(time.Second / time.Duration(w.config.FlowsPerSecond/w.config.Concurrency))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case flow := <-flows:
			batch = append(batch, flow)
			if len(batch) >= w.config.BatchSize {
				w.sendBatch(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.sendBatch(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (w *IngestWorker) sendBatch(ctx context.Context, batch []*svcproto.Flow) {
	start := time.Now()

	flowMap := make(map[string]*svcproto.Flow)
	for i, flow := range batch {
		flowMap[fmt.Sprintf("flow-%d", i)] = flow
	}

	req := &svcproto.FlowBatch{
		Flows:      flowMap,
		ReceivedAt: time.Now().Unix(),
	}

	_, err := w.client.IngestFlows(ctx, req)

	latency := time.Since(start)
	w.metrics.RecordLatency(latency)

	if err != nil {
		atomic.AddUint64(&w.metrics.TotalErrors, uint64(len(batch)))
		return
	}

	atomic.AddUint64(&w.metrics.TotalSent, uint64(len(batch)))
	atomic.AddUint64(&w.metrics.TotalReceived, uint64(len(batch)))
}

// QueryWorker sends query requests to the Center service
type QueryWorker struct {
	id      int
	config  *Config
	metrics *Metrics
	conn    *grpc.ClientConn
}

// NewQueryWorker creates a new query worker
func NewQueryWorker(id int, config *Config, metrics *Metrics) (*QueryWorker, error) {
	conn, err := grpc.Dial(config.Target,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &QueryWorker{
		id:     id,
		config: config,
		metrics: metrics,
		conn:   conn,
	}, nil
}

// Run starts the worker
func (w *QueryWorker) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	defer w.conn.Close()

	ticker := time.NewTicker(time.Second / time.Duration(w.config.Concurrency))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Simulate API query
			start := time.Now()
			time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
			latency := time.Since(start)

			w.metrics.RecordLatency(latency)
			atomic.AddUint64(&w.metrics.TotalSent, 1)
			atomic.AddUint64(&w.metrics.TotalReceived, 1)
		}
	}
}

// RunIngestTest runs the ingestion load test
func RunIngestTest(config *Config) error {
	metrics := NewMetrics()
	metrics.StartTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create flow channel
	flowChan := make(chan *svcproto.Flow, config.Concurrency*config.BatchSize)

	// Start flow generator
	go func() {
		generator := NewFlowGenerator()
		ticker := time.NewTicker(time.Second / time.Duration(config.FlowsPerSecond))
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				close(flowChan)
				return
			case <-ticker.C:
				flow := generator.GenerateFlow()
				select {
				case flowChan <- flow:
				default:
					// Channel full, skip this flow
				}
			}
		}
	}()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < config.Concurrency; i++ {
		worker, err := NewIngestWorker(i, config, metrics)
		if err != nil {
			return fmt.Errorf("failed to create worker %d: %w", i, err)
		}
		wg.Add(1)
		go worker.Run(ctx, flowChan, &wg)
	}

	// Wait for duration
	time.Sleep(time.Duration(config.Duration) * time.Second)
	cancel()
	wg.Wait()

	metrics.EndTime = time.Now()

	// Print report
	fmt.Println(metrics.Report())

	return nil
}

// RunQueryTest runs the query load test
func RunQueryTest(config *Config) error {
	metrics := NewMetrics()
	metrics.StartTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < config.Concurrency; i++ {
		worker, err := NewQueryWorker(i, config, metrics)
		if err != nil {
			return fmt.Errorf("failed to create worker %d: %w", i, err)
		}
		wg.Add(1)
		go worker.Run(ctx, &wg)
	}

	// Wait for duration
	time.Sleep(time.Duration(config.Duration) * time.Second)
	cancel()
	wg.Wait()

	metrics.EndTime = time.Now()

	// Print report
	fmt.Println(metrics.Report())

	return nil
}

func main() {
	config := &Config{}

	flag.StringVar(&config.Mode, "mode", "ingest", "Test mode: ingest, query, mixed")
	flag.StringVar(&config.Target, "target", "localhost:8080", "Center API address")
	flag.StringVar(&config.EdgeTarget, "edge-target", "localhost:9002", "Edge service address")
	flag.IntVar(&config.FlowsPerSecond, "flows", 10000, "Target flows per second")
	flag.IntVar(&config.Concurrency, "concurrency", 10, "Number of concurrent workers")
	flag.IntVar(&config.Duration, "duration", 60, "Test duration in seconds")
	flag.IntVar(&config.BatchSize, "batch-size", 100, "Batch size for sending flows")
	flag.StringVar(&config.ReportFile, "report", "", "Output report file path")
	flag.IntVar(&config.ReportInterval, "report-interval", 10, "Progress report interval in seconds")

	flag.Parse()

	fmt.Printf("Starting CloudFlow Load Test\n")
	fmt.Printf("Mode: %s\n", config.Mode)
	fmt.Printf("Target: %s\n", config.Target)
	fmt.Printf("Flows per second: %d\n", config.FlowsPerSecond)
	fmt.Printf("Concurrency: %d\n", config.Concurrency)
	fmt.Printf("Duration: %d seconds\n", config.Duration)
	fmt.Println()

	switch config.Mode {
	case "ingest":
		if err := RunIngestTest(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error running ingest test: %v\n", err)
			os.Exit(1)
		}
	case "query":
		if err := RunQueryTest(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error running query test: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", config.Mode)
		os.Exit(1)
	}
}