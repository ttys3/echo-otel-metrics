// Package otelmetric provides middleware to add opentelemetry metrics and Prometheus exporter.
package echootelmetrics

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	realprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Meter can be a global/package variable.
var meter = otel.GetMeterProvider().Meter("echo")

var (
	defaultMetricPath = "/metrics"
)

const (
	_           = iota // ignore first value by assigning to blank identifier
	_KB float64 = 1 << (10 * iota)
	_MB
	_GB
	_TB
)

const (
	unitDimensionless = "1"
	unitBytes         = "By"
	unitMilliseconds  = "ms"
)

// reqDurBucketsSeconds is the buckets for request duration. Here, we use the prometheus defaults
var reqDurBucketsSeconds = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// byteBuckets is the buckets for request/response size. Here we define a spectrom from 1KB thru 1NB up to 10MB.
var byteBuckets = []float64{1.0 * _KB, 2.0 * _KB, 5.0 * _KB, 10.0 * _KB, 100 * _KB, 500 * _KB, 1.0 * _MB, 2.5 * _MB, 5.0 * _MB, 10.0 * _MB}

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

// MiddlewareConfig contains the configuration for creating prometheus middleware collecting several default metrics.
type MiddlewareConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper

	ServiceName    string
	ServiceVersion string

	// Namespace is components of the fully-qualified name of the Metric (created by joining Namespace,Subsystem and Name components with "_")
	// this will take from ServiceName if not set
	// Optional
	Namespace string

	MetricsPath string

	RequestCounterURLLabelMappingFunc  RequestCounterLabelMappingFunc
	RequestCounterHostLabelMappingFunc RequestCounterLabelMappingFunc

	// if enabled, it will add the scope information (otel_scope_name="otelmetric-demo",otel_scope_version="") to every metrics
	WithScopeInfo bool

	// Registry is the prometheus registry that will be used as the default Registerer and
	// Gatherer if these are not specified.
	Registry *realprometheus.Registry

	// Registerer sets the prometheus.Registerer instance the middleware will register these metrics with.
	// Defaults to: prometheus.DefaultRegisterer
	Registerer realprometheus.Registerer

	// Gatherer is the prometheus gatherer to gather metrics with.
	// If not specified the Registry will be used as default.
	Gatherer realprometheus.Gatherer
}

// Prometheus contains the metrics gathered by the instance and its path
type Prometheus struct {
	reqCnt       metric.Int64Counter
	reqDur       metric.Float64Histogram
	reqSz, resSz metric.Int64Histogram
	router       *echo.Echo

	*MiddlewareConfig
}

// NewPrometheus generates a new set of metrics with a certain subsystem name
func NewPrometheus(config MiddlewareConfig) *Prometheus {
	if config.Skipper == nil {
		config.Skipper = middleware.DefaultSkipper
	}

	if config.MetricsPath == "" {
		config.MetricsPath = defaultMetricPath
	}

	if config.Registry != nil {
		config.Registerer = config.Registry
		config.Gatherer = config.Registry
	}

	if config.Registerer == nil {
		config.Registerer = realprometheus.DefaultRegisterer
	}
	if config.Gatherer == nil {
		config.Gatherer = realprometheus.DefaultGatherer
	}

	if config.RequestCounterURLLabelMappingFunc == nil {
		config.RequestCounterURLLabelMappingFunc = func(c echo.Context) string {
			// contains route path ala `/users/:id`
			// as of Echo v4.10.1 path is empty for 404 cases (when router did not find any matching routes)
			return c.Path()
		}
	}

	if config.RequestCounterHostLabelMappingFunc == nil {
		config.RequestCounterHostLabelMappingFunc = func(c echo.Context) string {
			return c.Request().Host
		}
	}

	p := &Prometheus{
		MiddlewareConfig: &config,
	}

	var err error
	// Standard default metrics
	p.reqCnt, err = meter.Int64Counter(
		// the result name is `requests_total`
		// https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/exporters/prometheus/exporter.go#L74
		// the exporter will enforce the `_total` suffix for counter, so we do not need it here
		"requests",
		// see https://github.com/open-telemetry/opentelemetry-go/pull/3776
		// The go.opentelemetry.io/otel/metric/unit package is deprecated. Setup the equivalent unit string instead. (#3776)
		// Setup "1" instead of unit.Dimensionless
		// Setup "By" instead of unit.Bytes
		// Setup "ms" instead of unit.Milliseconds

		// the exported metrics name will force suffix by unit, see
		// https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/exporters/prometheus/exporter.go#L315
		//
		//	var unitSuffixes = map[string]string{
		//		"1":  "_ratio",
		//		"By": "_bytes",
		//		"ms": "_milliseconds",
		//	}
		// disable this behaviour by using `prometheus.WithoutUnits()` option
		// or hack: do not set unit for counter to avoid the `_ratio` suffix
		metric.WithDescription("How many HTTP requests processed, partitioned by status code and HTTP method."),
	)
	if err != nil {
		panic(err)
	}

	p.reqDur, err = meter.Float64Histogram(
		"request_duration",
		metric.WithUnit("s"),
		metric.WithDescription("The HTTP request latencies in seconds."),
	)
	if err != nil {
		panic(err)
	}

	p.resSz, err = meter.Int64Histogram(
		"response_size",
		metric.WithUnit(unitBytes),
		metric.WithDescription("The HTTP response sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	p.reqSz, err = meter.Int64Histogram(
		"request_size",
		metric.WithUnit(unitBytes),
		metric.WithDescription("The HTTP request sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	p.initMetricsMeterProvider()
	return p
}

// SetMetricsExporterRoute set metrics paths
func (p *Prometheus) SetMetricsExporterRoute(e *echo.Echo) {
	e.GET(p.MetricsPath, p.ExporterHandler())
}

// Setup adds the middleware to the Echo engine.
func (p *Prometheus) Setup(e *echo.Echo) {
	e.Use(p.HandlerFunc)
	p.SetMetricsExporterRoute(e)
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

		elapsed := time.Since(start) / time.Millisecond

		url := p.RequestCounterURLLabelMappingFunc(c)
		host := p.RequestCounterHostLabelMappingFunc(c)

		elapsedSeconds := float64(elapsed) / float64(1000)
		p.reqDur.Record(c.Request().Context(), elapsedSeconds, metric.WithAttributes(
			attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("host", host),
			attribute.String("url", url)))

		// "code", "method", "host", "url"
		p.reqCnt.Add(c.Request().Context(), 1,
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", host),
				attribute.String("url", url)))

		p.reqSz.Record(c.Request().Context(), int64(reqSz),
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", host),
				attribute.String("url", url)))

		resSz := float64(c.Response().Size)
		p.resSz.Record(c.Request().Context(), int64(resSz),
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", host),
				attribute.String("url", url)))

		return err
	}
}

func (p *Prometheus) initMetricsMeterProvider() *prometheus.Exporter {
	namespace := p.Namespace
	if namespace == "" {
		namespace = p.ServiceName
	}
	namespace = strings.ReplaceAll(namespace, "-", "_")

	opts := []prometheus.Option{
		prometheus.WithRegisterer(p.Registerer),
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(p.ServiceName),
			semconv.ServiceVersion(p.ServiceVersion),
			semconv.ServiceNamespace(namespace),
		))
	if err != nil {
		panic(err)
	}

	if namespace != "" {
		opts = append(opts, prometheus.WithNamespace(namespace))
	}
	if !p.WithScopeInfo {
		opts = append(opts, prometheus.WithoutScopeInfo())
	}
	exporter, err := prometheus.New(opts...)
	if err != nil {
		panic(err)
	}

	durationBucketsView := sdkmetric.NewView(
		// TODO: support more views like:
		// request_duration_seconds
		// processing_time_seconds
		// latency_seconds
		// server_handling_seconds
		sdkmetric.Instrument{Name: "*request_duration"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
			Boundaries: reqDurBucketsSeconds,
		}},
	)

	reqBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*request_size"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
			Boundaries: byteBuckets,
		}},
	)

	rspBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*response_size"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
			Boundaries: byteBuckets,
		}},
	)

	defaultView := sdkmetric.NewView(sdkmetric.Instrument{Name: "*", Kind: sdkmetric.InstrumentKindCounter},
		sdkmetric.Stream{})

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		// view see https://github.com/open-telemetry/opentelemetry-go/blob/v1.11.2/exporters/prometheus/exporter_test.go#L291
		sdkmetric.WithReader(exporter),
		sdkmetric.WithView(durationBucketsView, reqBytesBucketsView, rspBytesBucketsView, defaultView),
	)

	otel.SetMeterProvider(provider)

	return exporter
}

func (p *Prometheus) ExporterHandler() echo.HandlerFunc {
	h := promhttp.HandlerFor(p.Gatherer, promhttp.HandlerOpts{})

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
