package main

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Meter can be a global/package variable.
// or use short https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/metric.go#LL35C6-L35C11
// Meter() which is short for GetMeterProvider().Meter(name)
var Meter = otel.GetMeterProvider().Meter(serviceName)

var demoCounter, _ = Meter.Int64Counter(
	"foobar",
	// metric.WithUnit("1"),
	metric.WithDescription("Just a test counter"),
)
