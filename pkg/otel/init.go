package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config OpenTelemetry配置
type Config struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	Endpoint       string `yaml:"endpoint" json:"endpoint"`
	ServiceName    string `yaml:"service_name" json:"service_name"`
	ServiceVersion string `yaml:"service_version" json:"service_version"`
	Environment    string `yaml:"environment" json:"environment"`
	Insecure       bool   `yaml:"insecure" json:"insecure"`
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

var (
	tracerProvider *sdktrace.TracerProvider
)

// Init 初始化OpenTelemetry
func Init(ctx context.Context, cfg Config) (func(), error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	// 创建资源
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

	// 创建Trace Exporter
	traceOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
	}

	traceExporter, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// 创建TracerProvider
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// 设置传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 返回清理函数
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			fmt.Printf("failed to shutdown tracer provider: %v\n", err)
		}
	}

	return cleanup, nil
}

// Tracer 获取Tracer
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// StartSpan 开始Span
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return tracerProvider.Tracer("cloudflow").Start(ctx, name)
}
