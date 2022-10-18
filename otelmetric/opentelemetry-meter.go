// Package otelmetric provides middleware to add opentelemetry metrics and Prometheus exporter.
package otelmetric

import (
	"errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument/syncfloat64"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/metric/unit"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"

	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"net/http"

	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"go.opentelemetry.io/otel/metric/instrument"
)

// Meter can be a global/package variable.
var meter = global.MeterProvider().Meter("echo")

var defaultMetricPath = "/metrics"
var defaultSubsystem = "echo"

const (
	_          = iota // ignore first value by assigning to blank identifier
	KB float64 = 1 << (10 * iota)
	MB
	GB
	TB
)

// reqDurBuckets is the buckets for request duration. Here, we use the prometheus defaults
// which are for ~10s request length max: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
var reqDurBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// reqSzBuckets is the buckets for request size. Here we define a spectrom from 1KB thru 1NB up to 10MB.
var reqSzBuckets = []float64{1.0 * KB, 2.0 * KB, 5.0 * KB, 10.0 * KB, 100 * KB, 500 * KB, 1.0 * MB, 2.5 * MB, 5.0 * MB, 10.0 * MB}

// resSzBuckets is the buckets for response size. Here we define a spectrom from 1KB thru 1NB up to 10MB.
var resSzBuckets = []float64{1.0 * KB, 2.0 * KB, 5.0 * KB, 10.0 * KB, 100 * KB, 500 * KB, 1.0 * MB, 2.5 * MB, 5.0 * MB, 10.0 * MB}

// Standard default metrics
//
//	counter, counter_vec, gauge, gauge_vec,
//	histogram, histogram_vec, summary, summary_vec
var reqCnt = &Metric{
	ID:          "reqCnt",
	Name:        "requests_total",
	Description: "How many HTTP requests processed, partitioned by status code and HTTP method.",
	Type:        "counter_vec",
	Unit:        unit.Dimensionless,
	Args:        []string{"code", "method", "host", "url"}}

var reqDur = &Metric{
	ID:          "reqDur",
	Name:        "request_duration_seconds",
	Description: "The HTTP request latencies in seconds.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        unit.Milliseconds,
	Buckets:     reqDurBuckets}

var resSz = &Metric{
	ID:          "resSz",
	Name:        "response_size_bytes",
	Description: "The HTTP response sizes in bytes.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        unit.Bytes,
	Buckets:     resSzBuckets}

var reqSz = &Metric{
	ID:          "reqSz",
	Name:        "request_size_bytes",
	Description: "The HTTP request sizes in bytes.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        unit.Bytes,
	Buckets:     reqSzBuckets}

var standardMetrics = []*Metric{
	reqCnt,
	reqDur,
	resSz,
	reqSz,
}

/*
RequestCounterLabelMappingFunc is a function which can be supplied to the middleware to control
the cardinality of the request counter's "url" label, which might be required in some contexts.
For instance, if for a "/customer/:name" route you don't want to generate a time series for every
possible customer name, you could use this function:

	func(c echo.Context) string {
		url := c.Request.URL.Path
		for _, p := range c.Params {
			if p.Key == "name" {
				url = strings.Replace(url, p.Value, ":name", 1)
				break
			}
		}
		return url
	}

which would map "/customer/alice" and "/customer/bob" to their template "/customer/:name".
It can also be applied for the "Host" label
*/
type RequestCounterLabelMappingFunc func(c echo.Context) string

// Metric is a definition for the name, description, type, ID, and
// prometheus.Collector type (i.e. CounterVec, Summary, etc) of each metric
type Metric struct {
	ID          string
	Name        string
	Description string
	Type        string
	Unit        unit.Unit
	Args        []string
	Buckets     []float64
}

// Prometheus contains the metrics gathered by the instance and its path
type Prometheus struct {
	reqCnt               syncint64.Counter
	reqDur, reqSz, resSz syncfloat64.Histogram
	router               *echo.Echo
	listenAddress        string

	MetricsList []*Metric
	MetricsPath string
	Subsystem   string
	Skipper     middleware.Skipper

	RequestCounterURLLabelMappingFunc  RequestCounterLabelMappingFunc
	RequestCounterHostLabelMappingFunc RequestCounterLabelMappingFunc

	// Context string to use as a prometheus URL label
	URLLabelFromContext string
}

// NewPrometheus generates a new set of metrics with a certain subsystem name
func NewPrometheus(subsystem string, skipper middleware.Skipper) *Prometheus {
	if skipper == nil {
		skipper = middleware.DefaultSkipper
	}

	if subsystem == "" {
		subsystem = defaultSubsystem
	}

	p := &Prometheus{
		MetricsList: standardMetrics,
		MetricsPath: defaultMetricPath,
		Subsystem:   subsystem,
		Skipper:     skipper,
		RequestCounterURLLabelMappingFunc: func(c echo.Context) string {
			return c.Path() // i.e. by default do nothing, i.e. return URL as is
		},
		RequestCounterHostLabelMappingFunc: func(c echo.Context) string {
			return c.Request().Host
		},
	}

	p.registerMetrics(subsystem)

	return p
}

// SetListenAddress for exposing metrics on address. If not set, it will be exposed at the
// same address of the echo engine that is being used
// func (p *Prometheus) SetListenAddress(address string) {
// 	p.listenAddress = address
// 	if p.listenAddress != "" {
// 		p.router = echo.Echo().Router()
// 	}
// }

// SetListenAddressWithRouter for using a separate router to expose metrics. (this keeps things like GET /metrics out of
// your content's access log).
// func (p *Prometheus) SetListenAddressWithRouter(listenAddress string, r *echo.Echo) {
// 	p.listenAddress = listenAddress
// 	if len(p.listenAddress) > 0 {
// 		p.router = r
// 	}
// }

// SetMetricsPath set metrics paths
func (p *Prometheus) SetMetricsPath(e *echo.Echo) {
	if p.listenAddress != "" {
		p.router.GET(p.MetricsPath, p.prometheusHandler())
		p.runServer()
	} else {
		e.GET(p.MetricsPath, p.prometheusHandler())
	}
}

func (p *Prometheus) runServer() {
	if p.listenAddress != "" {
		go p.router.Start(p.listenAddress)
	}
}

func (p *Prometheus) registerMetrics(subsystem string) {
	for _, metricDef := range p.MetricsList {
		switch metricDef {
		case reqCnt:
			p.reqCnt, _ = meter.SyncInt64().Counter(
				subsystem+"."+metricDef.Name,
				instrument.WithUnit(metricDef.Unit),
				instrument.WithDescription(metricDef.Description),
			)
		case reqDur:
			p.reqDur, _ = meter.SyncFloat64().Histogram(
				subsystem+"."+metricDef.Name,
				instrument.WithUnit(metricDef.Unit),
				instrument.WithDescription(metricDef.Description),
			)
		case resSz:
			p.resSz, _ = meter.SyncFloat64().Histogram(
				subsystem+"."+metricDef.Name,
				instrument.WithUnit(metricDef.Unit),
				instrument.WithDescription(metricDef.Description),
			)
		case reqSz:
			p.reqSz, _ = meter.SyncFloat64().Histogram(
				subsystem+"."+metricDef.Name,
				instrument.WithUnit(metricDef.Unit),
				instrument.WithDescription(metricDef.Description),
			)
		}
	}
}

// Use adds the middleware to the Echo engine.
func (p *Prometheus) Use(e *echo.Echo) {
	e.Use(p.HandlerFunc)
	p.SetMetricsPath(e)
}

// HandlerFunc defines handler function for middleware
func (p *Prometheus) HandlerFunc(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Path() == p.MetricsPath {
			return next(c)
		}
		if p.Skipper(c) {
			return next(c)
		}

		start := time.Now()
		reqSz := computeApproximateRequestSize(c.Request())

		err := next(c)

		status := c.Response().Status
		if err != nil {
			var httpError *echo.HTTPError
			if errors.As(err, &httpError) {
				status = httpError.Code
			}
			if status == 0 || status == http.StatusOK {
				status = http.StatusInternalServerError
			}
		}

		elapsed := float64(time.Since(start)) / float64(time.Millisecond)

		url := p.RequestCounterURLLabelMappingFunc(c)
		if len(p.URLLabelFromContext) > 0 {
			u := c.Get(p.URLLabelFromContext)
			if u == nil {
				u = "unknown"
			}
			url = u.(string)
		}

		p.reqDur.Record(c.Request().Context(), elapsed,
			attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("url", url))

		// "code", "method", "host", "url"
		p.reqCnt.Add(c.Request().Context(), 1,
			attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("host", p.RequestCounterHostLabelMappingFunc(c)),
			attribute.String("url", url))

		p.reqSz.Record(c.Request().Context(), float64(reqSz), attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("url", url))

		resSz := float64(c.Response().Size)
		p.resSz.Record(c.Request().Context(), resSz, attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("url", url))

		return err
	}
}

func configureMetrics(serviceName string) *prometheus.Exporter {
	config := prometheus.Config{
		DefaultHistogramBoundaries: reqDurBuckets,
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(semconv.ServiceNameKey.String(serviceName)))
	if err != nil {
		panic(err)
	}

	ctrl := controller.New(
		processor.NewFactory(
			// selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
			// selector.NewWithHistogramDistribution(
			// 	histogram.WithExplicitBoundaries(config.DefaultHistogramBoundaries),
			// ),
			// we need set different boundaries for different unit, so comes with our own selector
			NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(config.DefaultHistogramBoundaries),
			),
			aggregation.CumulativeTemporalitySelector(),
			processor.WithMemory(true),
		),
		controller.WithResource(res),
	)

	exporter, err := prometheus.New(config, ctrl)
	if err != nil {
		panic(err)
	}

	global.SetMeterProvider(exporter.MeterProvider())

	return exporter
}

func (p *Prometheus) prometheusHandler() echo.HandlerFunc {
	h := configureMetrics(p.Subsystem)
	return func(c echo.Context) error {
		h.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

func computeApproximateRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s = len(r.URL.Path)
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	// N.B. r.Form and r.MultipartForm are assumed to be included in r.URL.

	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}
