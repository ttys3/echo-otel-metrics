// Package otelmetric provides middleware to add opentelemetry metrics and Prometheus exporter.
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
var meter = otel.GetMeterProvider().Meter("echo")

var (
	defaultMetricPath = "/metrics"
	defaultSubsystem  = "echo"
)

const (
	_          = iota // ignore first value by assigning to blank identifier
	KB float64 = 1 << (10 * iota)
	MB
	GB
	TB
)

const (
	UnitDimensionless = "1"
	UnitBytes         = "By"
	UnitMilliseconds  = "ms"
)

// reqDurBuckets is the buckets for request duration. Here, we use the prometheus defaults
// which are for ~10s request length max: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
var reqDurBuckets = []float64{.005 * 1000, .01 * 1000, .025 * 1000, .05 * 1000, .1 * 1000, .25 * 1000, .5 * 1000, 1 * 1000, 2.5 * 1000, 5 * 1000, 10 * 1000}

// reqSzBuckets is the buckets for request size. Here we define a spectrom from 1KB thru 1NB up to 10MB.
var reqSzBuckets = []float64{1.0 * KB, 2.0 * KB, 5.0 * KB, 10.0 * KB, 100 * KB, 500 * KB, 1.0 * MB, 2.5 * MB, 5.0 * MB, 10.0 * MB}

// resSzBuckets is the buckets for response size. Here we define a spectrom from 1KB thru 1NB up to 10MB.
var resSzBuckets = []float64{1.0 * KB, 2.0 * KB, 5.0 * KB, 10.0 * KB, 100 * KB, 500 * KB, 1.0 * MB, 2.5 * MB, 5.0 * MB, 10.0 * MB}

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

// Prometheus contains the metrics gathered by the instance and its path
type Prometheus struct {
	reqCnt               metric.Int64Counter
	reqDur, reqSz, resSz metric.Float64Histogram
	router               *echo.Echo
	listenAddress        string
	compatibleMode       bool // run as echo prometheus middleware compatible mode

	MetricsPath    string
	Subsystem      string
	ServiceVersion string
	Skipper        middleware.Skipper

	RequestCounterURLLabelMappingFunc  RequestCounterLabelMappingFunc
	RequestCounterHostLabelMappingFunc RequestCounterLabelMappingFunc

	// Registry is the prometheus registry that will be used as the default Registerer and
	// Gatherer if these are not specified.
	Registry *realprometheus.Registry

	Registerer realprometheus.Registerer

	// Gatherer is the prometheus gatherer to gather
	// metrics with.
	//
	// If not specified the Registry will be used as default.
	Gatherer realprometheus.Gatherer
}

// NewPrometheus generates a new set of metrics with a certain subsystem name
func NewPrometheus(subsystem string, serviceVersion string, skipper middleware.Skipper, compatibleMode bool) *Prometheus {
	if skipper == nil {
		skipper = middleware.DefaultSkipper
	}

	if subsystem == "" {
		subsystem = defaultSubsystem
	}

	registry := realprometheus.NewRegistry()

	p := &Prometheus{
		compatibleMode: compatibleMode,
		MetricsPath:    defaultMetricPath,
		Subsystem:      subsystem,
		ServiceVersion: serviceVersion,
		Skipper:        skipper,
		Registry:       registry,
		Registerer:     registry,
		Gatherer:       registry,
		RequestCounterURLLabelMappingFunc: func(c echo.Context) string {
			// contains route path ala `/users/:id`
			// as of Echo v4.10.1 path is empty for 404 cases (when router did not find any matching routes)
			return c.Path()
		},
		RequestCounterHostLabelMappingFunc: func(c echo.Context) string {
			return c.Request().Host
		},
	}

	var err error
	// Standard default metrics
	p.reqCnt, err = meter.Int64Counter(
		// the result name is `requests_total`
		// https://github.com/open-telemetry/opentelemetry-go/blob/46f2ce5ca6adaa264c37cdbba251c9184a06ed7f/exporters/prometheus/exporter.go#L74
		// the exporter will enforce the `_total` suffix for counter, so we do not need it here
		"requests",
		// see https://github.com/open-telemetry/opentelemetry-go/pull/3776
		// The go.opentelemetry.io/otel/metric/unit package is deprecated. Use the equivalent unit string instead. (#3776)
		// Use "1" instead of unit.Dimensionless
		// Use "By" instead of unit.Bytes
		// Use "ms" instead of unit.Milliseconds

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
		// metric.WithUnit(UnitDimensionless),
		metric.WithDescription("How many HTTP requests processed, partitioned by status code and HTTP method."),
	)

	if err != nil {
		panic(err)
	}

	reqDurName := "request_duration"
	if p.compatibleMode {
		reqDurName = "request_duration_seconds"
	}
	p.reqDur, err = meter.Float64Histogram(
		reqDurName,
		metric.WithUnit(UnitMilliseconds),
		metric.WithDescription("The HTTP request latencies in milliseconds."),
	)
	if err != nil {
		panic(err)
	}

	resSzName := "response_size"
	if p.compatibleMode {
		resSzName = "response_size_bytes"
	}
	p.resSz, err = meter.Float64Histogram(
		resSzName,
		metric.WithUnit(UnitBytes),
		metric.WithDescription("The HTTP response sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	reqSzName := "request_size"
	if p.compatibleMode {
		reqSzName = "request_size_bytes"
	}
	p.reqSz, err = meter.Float64Histogram(
		reqSzName,
		metric.WithUnit(UnitBytes),
		metric.WithDescription("The HTTP request sizes in bytes."),
	)
	if err != nil {
		panic(err)
	}

	return p
}

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
		// nolint: errcheck
		go p.router.Start(p.listenAddress)
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
		if p.compatibleMode {
			elapsed = elapsed / 1000
		}

		url := p.RequestCounterURLLabelMappingFunc(c)
		host := p.RequestCounterHostLabelMappingFunc(c)

		p.reqDur.Record(c.Request().Context(), elapsed, metric.WithAttributes(
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

		p.reqSz.Record(c.Request().Context(), float64(reqSz),
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", host),
				attribute.String("url", url)))

		resSz := float64(c.Response().Size)
		p.resSz.Record(c.Request().Context(), resSz,
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", host),
				attribute.String("url", url)))

		return err
	}
}

func (p *Prometheus) configureMetrics() *prometheus.Exporter {
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
	if p.compatibleMode {
		opts = append(opts, prometheus.WithoutUnits())
		opts = append(opts, prometheus.WithoutScopeInfo())
	}
	exporter, err := prometheus.New(opts...)
	if err != nil {
		panic(err)
	}

	durationBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*_duration"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
			Boundaries: reqDurBuckets,
		}},
	)

	reqBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*request_size"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
			Boundaries: reqSzBuckets,
		}},
	)

	rspBytesBucketsView := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "*response_size"},
		sdkmetric.Stream{Aggregation: aggregation.ExplicitBucketHistogram{
			Boundaries: resSzBuckets,
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

func (p *Prometheus) prometheusHandler() echo.HandlerFunc {
	p.configureMetrics()

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
