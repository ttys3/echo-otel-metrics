package otelmetric

import (
	"go.opentelemetry.io/otel/metric/unit"
	"go.opentelemetry.io/otel/sdk/metric/aggregator"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/lastvalue"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/sum"
	"go.opentelemetry.io/otel/sdk/metric/export"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
)

type (
	selectorHistogram struct {
		options []histogram.Option
	}
)

var (
	_ export.AggregatorSelector = selectorHistogram{}
)

// NewWithHistogramDistribution returns a simple aggregator selector
// that uses histogram aggregators for `Histogram` instruments.
// This selector is a good default choice for most metric exporters.
func NewWithHistogramDistribution(options ...histogram.Option) export.AggregatorSelector {
	return selectorHistogram{options: options}
}

func sumAggs(aggPtrs []*aggregator.Aggregator) {
	aggs := sum.New(len(aggPtrs))
	for i := range aggPtrs {
		*aggPtrs[i] = &aggs[i]
	}
}

func lastValueAggs(aggPtrs []*aggregator.Aggregator) {
	aggs := lastvalue.New(len(aggPtrs))
	for i := range aggPtrs {
		*aggPtrs[i] = &aggs[i]
	}
}

func (s selectorHistogram) AggregatorFor(descriptor *sdkapi.Descriptor, aggPtrs ...*aggregator.Aggregator) {
	switch descriptor.InstrumentKind() {
	case sdkapi.GaugeObserverInstrumentKind:
		lastValueAggs(aggPtrs)
	case sdkapi.HistogramInstrumentKind:
		if descriptor.Unit() == unit.Milliseconds {
			s.options = []histogram.Option{
				histogram.WithExplicitBoundaries(reqDurBuckets),
			}
		} else if descriptor.Unit() == unit.Bytes {
			s.options = []histogram.Option{
				histogram.WithExplicitBoundaries(reqSzBuckets),
			}
		}
		aggs := histogram.New(len(aggPtrs), descriptor, s.options...)
		for i := range aggPtrs {
			*aggPtrs[i] = &aggs[i]
		}
	default:
		sumAggs(aggPtrs)
	}
}
