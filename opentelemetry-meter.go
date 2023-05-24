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

// Standard default metrics
//
//	counter, counter_vec, gauge, gauge_vec,
//	histogram, histogram_vec, summary, summary_vec
var reqCnt = &Metric{
	ID:          "reqCnt",
	Name:        "requests_total",
	Description: "How many HTTP requests processed, partitioned by status code and HTTP method.",
	Type:        "counter_vec",
	// see https://github.com/open-telemetry/opentelemetry-go/pull/3776
	// The go.opentelemetry.io/otel/metric/unit package is deprecated. Use the equivalent unit string instead. (#3776)
	// Use "1" instead of unit.Dimensionless
	// Use "By" instead of unit.Bytes
	// Use "ms" instead of unit.Milliseconds
	Unit: UnitDimensionless,
	Args: []string{"code", "method", "host", "url"},
}

var reqDur = &Metric{
	ID:          "reqDur",
	Name:        "request_duration",
	Description: "The HTTP request latencies in seconds.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        UnitMilliseconds,
	Buckets:     reqDurBuckets,
}

var resSz = &Metric{
	ID:          "resSz",
	Name:        "response_size",
	Description: "The HTTP response sizes in bytes.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        UnitBytes,
	Buckets:     resSzBuckets,
}

var reqSz = &Metric{
	ID:          "reqSz",
	Name:        "request_size",
	Description: "The HTTP request sizes in bytes.",
	Args:        []string{"code", "method", "url"},
	Type:        "histogram_vec",
	Unit:        UnitBytes,
	Buckets:     reqSzBuckets,
}

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
	Unit        string
	Args        []string
	Buckets     []float64
}

// Prometheus contains the metrics gathered by the instance and its path
type Prometheus struct {
	reqCnt               metric.Int64Counter
	reqDur, reqSz, resSz metric.Float64Histogram
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
func NewPrometheus(subsystem string, skipper middleware.Skipper) *Prometheus {
	if skipper == nil {
		skipper = middleware.DefaultSkipper
	}

	if subsystem == "" {
		subsystem = defaultSubsystem
	}

	registry := realprometheus.NewRegistry()

	p := &Prometheus{
		MetricsList: standardMetrics,
		MetricsPath: defaultMetricPath,
		Subsystem:   subsystem,
		Skipper:     skipper,
		Registry:    registry,
		Registerer:  registry,
		Gatherer:    registry,
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
		// nolint: errcheck
		go p.router.Start(p.listenAddress)
	}
}

func (p *Prometheus) registerMetrics(subsystem string) {
	for _, metricDef := range p.MetricsList {
		switch metricDef {
		case reqCnt:
			p.reqCnt, _ = meter.Int64Counter(
				subsystem+"."+metricDef.Name,
				metric.WithUnit(metricDef.Unit),
				metric.WithDescription(metricDef.Description),
			)
		case reqDur:
			p.reqDur, _ = meter.Float64Histogram(
				subsystem+"."+metricDef.Name,
				metric.WithUnit(metricDef.Unit),
				metric.WithDescription(metricDef.Description),
			)
		case resSz:
			p.resSz, _ = meter.Float64Histogram(
				subsystem+"."+metricDef.Name,
				metric.WithUnit(metricDef.Unit),
				metric.WithDescription(metricDef.Description),
			)
		case reqSz:
			p.reqSz, _ = meter.Float64Histogram(
				subsystem+"."+metricDef.Name,
				metric.WithUnit(metricDef.Unit),
				metric.WithDescription(metricDef.Description),
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

		p.reqDur.Record(c.Request().Context(), elapsed, metric.WithAttributes(
			attribute.Int("code", status),
			attribute.String("method", c.Request().Method),
			attribute.String("url", url)))

		// "code", "method", "host", "url"
		p.reqCnt.Add(c.Request().Context(), 1,
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("host", p.RequestCounterHostLabelMappingFunc(c)),
				attribute.String("url", url)))

		p.reqSz.Record(c.Request().Context(), float64(reqSz),
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("url", url)))

		resSz := float64(c.Response().Size)
		p.resSz.Record(c.Request().Context(), resSz,
			metric.WithAttributes(
				attribute.Int("code", status),
				attribute.String("method", c.Request().Method),
				attribute.String("url", url)))

		return err
	}
}

func configureMetrics(reg realprometheus.Registerer, serviceName string) *prometheus.Exporter {
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(semconv.ServiceNameKey.String(serviceName)))
	if err != nil {
		panic(err)
	}
	exporter, err := prometheus.New(
		prometheus.WithRegisterer(reg),
	)
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
		sdkmetric.Stream{Name: "*"})

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
	configureMetrics(p.Registerer, p.Subsystem)

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
