# CloudFlow Performance Testing Makefile

.PHONY: help test test-all test-ingest-50k test-ingest-100k test-ingest-150k test-api test-mixed clean report

# Default target
help:
	@echo "CloudFlow Performance Testing"
	@echo ""
	@echo "Available targets:"
	@echo "  test-all         - Run all performance tests"
	@echo "  test-ingest-50k  - Test ingestion at 50K flows/sec"
	@echo "  test-ingest-100k - Test ingestion at 100K flows/sec"
	@echo "  test-ingest-150k - Test ingestion at 150K flows/sec"
	@echo "  test-api         - Test API query performance"
	@echo "  test-mixed       - Test mixed load scenario"
	@echo "  clean            - Clean test reports"
	@echo "  report           - Generate performance report"

# Build load tester
build:
	@echo "Building load tester..."
	cd tools/load-tester && go build -o load-tester .

# Test ingestion at 50K flows/sec
test-ingest-50k: build
	@echo "Running 50K flows/sec ingestion test..."
	@./tools/load-tester/load-tester \
		--mode=ingest \
		--edge-target=localhost:9002 \
		--flows=50000 \
		--concurrency=10 \
		--duration=300 \
		--batch-size=100 \
		--report=reports/ingest-50k.json

# Test ingestion at 100K flows/sec
test-ingest-100k: build
	@echo "Running 100K flows/sec ingestion test..."
	@./tools/load-tester/load-tester \
		--mode=ingest \
		--edge-target=localhost:9002 \
		--flows=100000 \
		--concurrency=20 \
		--duration=300 \
		--batch-size=100 \
		--report=reports/ingest-100k.json

# Test ingestion at 150K flows/sec
test-ingest-150k: build
	@echo "Running 150K flows/sec ingestion test..."
	@./tools/load-tester/load-tester \
		--mode=ingest \
		--edge-target=localhost:9002 \
		--flows=150000 \
		--concurrency=30 \
		--duration=300 \
		--batch-size=100 \
		--report=reports/ingest-150k.json

# Test API query performance
test-api: build
	@echo "Running API query test..."
	@./tools/load-tester/load-tester \
		--mode=query \
		--target=localhost:8080 \
		--concurrency=100 \
		--duration=300 \
		--report=reports/api-query.json

# Test mixed load
test-mixed: build
	@echo "Running mixed load test..."
	@./tools/load-tester/load-tester \
		--mode=mixed \
		--target=localhost:8080 \
		--edge-target=localhost:9002 \
		--flows=80000 \
		--concurrency=100 \
		--duration=600 \
		--report=reports/mixed-load.json

# Run all tests
test-all: test-ingest-50k test-ingest-100k test-api test-mixed
	@echo "All tests completed!"
	@$(MAKE) report

# Generate performance report
report:
	@echo "Generating performance report..."
	@mkdir -p reports
	@echo "# CloudFlow Performance Test Results" > reports/results.md
	@echo "" >> reports/results.md
	@echo "Generated: $$(date)" >> reports/results.md
	@echo "" >> reports/results.md
	@if [ -f reports/ingest-50k.json ]; then \
		echo "## 50K flows/sec Test" >> reports/results.md; \
		cat reports/ingest-50k.json >> reports/results.md; \
		echo "" >> reports/results.md; \
	fi
	@if [ -f reports/ingest-100k.json ]; then \
		echo "## 100K flows/sec Test" >> reports/results.md; \
		cat reports/ingest-100k.json >> reports/results.md; \
		echo "" >> reports/results.md; \
	fi
	@if [ -f reports/api-query.json ]; then \
		echo "## API Query Test" >> reports/results.md; \
		cat reports/api-query.json >> reports/results.md; \
		echo "" >> reports/results.md; \
	fi
	@echo "Report generated: reports/results.md"

# Clean test reports
clean:
	@echo "Cleaning test reports..."
	@rm -rf reports/*.json reports/*.md
	@echo "Cleaned!"

# Run quick smoke test
test-smoke: build
	@echo "Running smoke test (10K flows/sec, 30s)..."
	@./tools/load-tester/load-tester \
		--mode=ingest \
		--edge-target=localhost:9002 \
		--flows=10000 \
		--concurrency=5 \
		--duration=30 \
		--batch-size=100

.DEFAULT_GOAL := help