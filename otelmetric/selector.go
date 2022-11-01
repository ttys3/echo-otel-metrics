package otelmetric

import (
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
	"go.opentelemetry.io/otel/sdk/metric/view"
)

func CustomSelector(ik view.InstrumentKind) aggregation.Aggregation {
	switch ik {
	case view.SyncCounter, view.SyncUpDownCounter, view.AsyncCounter, view.AsyncUpDownCounter:
		return aggregation.Sum{}
	case view.AsyncGauge:
		return aggregation.LastValue{}
	case view.SyncHistogram:
		return aggregation.ExplicitBucketHistogram{
			Boundaries: reqDurBuckets,
			NoMinMax:   false,
		}
	}
	panic("unknown instrument kind")
}
