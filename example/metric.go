package main

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// AppMeter can be a global/package variable.
// or use short https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/metric.go#LL35C6-L35C11
// Meter() which is short for GetMeterProvider().Meter(name)
var AppMeter = otel.Meter(serviceName, metric.WithInstrumentationVersion(serviceVersion))

var demoCounter, _ = AppMeter.Int64Counter(
	"foobar",
	metric.WithDescription("Just a test counter"),
)
