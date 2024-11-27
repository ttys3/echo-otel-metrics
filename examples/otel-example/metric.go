package main

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var shortExecBucketsSeconds = []float64{.002, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
var longExecBucketsSeconds = []float64{0.5, 1.0, 1.5, 2.5, 5.0, 10.0, 15.0, 25.0, 40.0, 60, 90, 120, 150, 200, 250, 300}

// Meter can be a global/package variable.
// or use short https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/metric.go#LL35C6-L35C11
// Meter() which is short for GetMeterProvider().Meter(name)
var Meter = otel.GetMeterProvider().Meter(serviceName)

var demoCounter, _ = Meter.Int64Counter(
	"foobar",
	metric.WithDescription("Just a test counter"),
)

var execCostTimeHistogram, _ = Meter.Float64Histogram("my.exec.cost",
	metric.WithUnit("s"),
	metric.WithDescription("exec time cost in seconds"),
	metric.WithExplicitBucketBoundaries(shortExecBucketsSeconds...),
)

var longExecCostTimeHistogram, _ = Meter.Float64Histogram("my.long_exec.cost",
	metric.WithUnit("s"),
	metric.WithDescription("exec time cost in seconds for long running tasks"),
	metric.WithExplicitBucketBoundaries(longExecBucketsSeconds...),
)
