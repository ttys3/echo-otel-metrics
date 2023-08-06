// Package otelmetric provides middleware to add opentelemetry metrics and OtelMetrics exporter.
package echootelmetrics

import (
	"errors"
	"go.opentelemetry.io/otel"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	realprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Meter can be a global/package variable.
var meter = otel.GetMeterProvider().Meter("github.com/ttys3/echo-otel-metrics/v2")

var (
	defaultMetricPath = "/metrics"
	defaultSubsystem  = "echo"
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
	unitSecond        = "s"
)

// as https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpserverrequestduration spec
// This metric SHOULD be specified with ExplicitBucketBoundaries of [ 0, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10 ].
var reqDurBucketsSeconds = []float64{0, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

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

	// Namespace is components of the fully-qualified name of the Metric (created by joining Namespace,Subsystem and Name components with "_")
	// Optional
	Namespace string

	// Subsystem is components of the fully-qualified name of the Metric (created by joining Namespace,Subsystem and Name components with "_")
	// Defaults to: "echo"
	Subsystem string

	ServiceVersion string

	MetricsPath string

	RequestCounterURLLabelMappingFunc  RequestCounterLabelMappingFunc
	RequestCounterHostLabelMappingFunc RequestCounterLabelMappingFunc

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

// OtelMetrics contains the metrics gathered by the instance and its path
type OtelMetrics struct {
	reqCnt       metric.Int64Counter
	reqDur       metric.Float64Histogram
	reqSz, resSz metric.Int64Histogram
	reqActive    metric.Int64UpDownCounter
	router       *echo.Echo

	*MiddlewareConfig
}

// New generates a new set of metrics with a certain subsystem name
func New(config MiddlewareConfig) *OtelMetrics {
	if config.Skipper == nil {
		config.Skipper = middleware.DefaultSkipper
	}

	if config.Subsystem == "" {
		config.Subsystem = defaultSubsystem
	}

	if config.MetricsPath == "" {
		config.MetricsPath = defaultMetricPath
	}

	registry := realprometheus.NewRegistry()

	if config.Registry == nil {
		config.Registry = registry
	}

	if config.Registerer == nil {
		config.Registerer = registry
	}
	if config.Gatherer == nil {
		config.Gatherer = registry
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

	p := &OtelMetrics{
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
		"http.server.request.duration",
		metric.WithUnit(unitSecond),
		metric.WithDescription("Measures the duration of inbound HTTP requests."),
	)
	if err != nil {
		panic(err)
	}

	p.resSz, err = meter.Int64Histogram(
		"http.server.response.size",
		metric.WithUnit(unitBytes),
		metric.WithDescription("The HTTP response sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	p.reqSz, err = meter.Int64Histogram(
		"http.server.request.size",
		metric.WithUnit(unitBytes),
		metric.WithDescription("The HTTP request sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	p.reqActive, err = meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithDescription("The current number of active requests."),
	)
	if err != nil {
		panic(err)
	}

	return p
}

// SetMetricsExporterRoute set metrics paths
func (p *OtelMetrics) SetMetricsExporterRoute(e *echo.Echo) {
	e.GET(p.MetricsPath, p.ExporterHandler())
}

// Setup adds the middleware to the Echo engine.
func (p *OtelMetrics) Setup(e *echo.Echo) {
	e.Use(p.HandlerFunc)
	p.SetMetricsExporterRoute(e)
}

// HandlerFunc defines handler function for middleware
func (p *OtelMetrics) HandlerFunc(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Path() == p.MetricsPath {
			return next(c)
		}
		if p.Skipper(c) {
			return next(c)
		}

		method := attribute.String("http.request.method", c.Request().Method)
		scheme := attribute.String("url.scheme", c.Scheme())
		host := p.RequestCounterHostLabelMappingFunc(c)
		serverAddress := attribute.String("server.address", host)
		p.reqActive.Add(c.Request().Context(), 1, metric.WithAttributes(method, scheme, serverAddress))

		reqSz := computeApproximateRequestSize(c.Request())

		start := time.Now()

		err := next(c)

		p.reqActive.Add(c.Request().Context(), -1, metric.WithAttributes(method, scheme, serverAddress))

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
		statusCode := attribute.Int("http.response.status_code", status)

		elapsedSeconds := float64(int(time.Since(start)/time.Millisecond)) / 1e3

		route := p.RequestCounterURLLabelMappingFunc(c)
		httpRoute := attribute.String("http.route", route)

		p.reqDur.Record(c.Request().Context(), elapsedSeconds, metric.WithAttributes(
			statusCode,
			method,
			serverAddress,
			httpRoute,
			scheme))

		p.reqCnt.Add(c.Request().Context(), 1,
			metric.WithAttributes(
				statusCode,
				method,
				serverAddress,
				httpRoute,
				scheme))

		p.reqSz.Record(c.Request().Context(), int64(reqSz),
			metric.WithAttributes(
				statusCode,
				method,
				serverAddress,
				httpRoute,
				scheme))

		resSz := float64(c.Response().Size)
		p.resSz.Record(c.Request().Context(), int64(resSz),
			metric.WithAttributes(
				statusCode,
				method,
				serverAddress,
				httpRoute,
				scheme))

		return err
	}
}

func (p *OtelMetrics) initMetricsExporter() *prometheus.Exporter {
	serviceName := p.Subsystem
	res, err := resource.Merge(resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(p.ServiceVersion),
		))
	if err != nil {
		panic(err)
	}

	opts := []prometheus.Option{
		prometheus.WithRegisterer(p.Registerer),
		prometheus.WithNamespace(serviceName),
	}
	exporter, err := prometheus.New(opts...)
	if err != nil {
		panic(err)
	}

	durationBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*request.duration"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
			Boundaries: reqDurBucketsSeconds,
		}},
	)

	reqBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*request.size"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
			Boundaries: byteBuckets,
		}},
	)

	rspBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*response.size"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
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

func (p *OtelMetrics) ExporterHandler() echo.HandlerFunc {
	p.initMetricsExporter()

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
