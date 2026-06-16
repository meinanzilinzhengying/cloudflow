package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// Config OpenTelemetry配置
type Config struct {
	Enabled        bool   `yaml:"enabled"`
	Endpoint       string `yaml:"endpoint"`
	ServiceName    string `yaml:"service_name"`
	ServiceVersion string `yaml:"service_version"`
	Environment    string `yaml:"environment"`
	Insecure       bool   `yaml:"insecure"`
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		Endpoint:       "http://localhost:4318",
		ServiceName:    "cloudflow",
		ServiceVersion: "1.0.0",
		Environment:    "production",
		Insecure:       true,
	}
}

// Init 初始化OpenTelemetry
func Init(ctx context.Context, cfg Config) (func(), error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 设置传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 初始化TracerProvider
	traceShutdown, err := initTracer(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to init tracer: %w", err)
	}

	// 初始化MeterProvider
	metricShutdown, err := initMeter(ctx, cfg, res)
	if err != nil {
		traceShutdown()
		return nil, fmt.Errorf("failed to init meter: %w", err)
	}

	// 初始化LoggerProvider
	logShutdown, err := initLogger(ctx, cfg, res)
	if err != nil {
		traceShutdown()
		metricShutdown()
		return nil, fmt.Errorf("failed to init logger: %w", err)
	}

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		logShutdown(ctx)
		metricShutdown(ctx)
		traceShutdown(ctx)
	}

	return shutdown, nil
}

// initTracer 初始化追踪
func initTracer(ctx context.Context, cfg Config, res *resource.Resource) (func(context.Context) error, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// initMeter 初始化指标
func initMeter(ctx context.Context, cfg Config, res *resource.Resource) (func(context.Context) error, error) {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}

// initLogger 初始化日志
func initLogger(ctx context.Context, cfg Config, res *resource.Resource) (func(context.Context) error, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	exporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)

	global.SetLoggerProvider(lp)

	return lp.Shutdown, nil
}
