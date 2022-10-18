package main

import "go.opentelemetry.io/otel/metric/instrument"
import "go.opentelemetry.io/otel/metric/global"

// Meter can be a global/package variable.
var Meter = global.MeterProvider().Meter(serviceName)

var demoCounter, _ = Meter.SyncInt64().Counter(
	serviceName+".my_counter",
	instrument.WithUnit("1"),
	instrument.WithDescription("Just a test counter"),
)
