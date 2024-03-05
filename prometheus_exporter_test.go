package echootelmetrics

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompModeCustomRegistryMetricsDoNotRecord404Route(t *testing.T) {
	e := echo.New()

	customRegistry := prometheus.NewRegistry()

	prom := New(MiddlewareConfig{Registry: customRegistry})
	e.Use(prom.Middleware())
	e.GET("/metrics", prom.ExporterHandler())

	assert.Equal(t, http.StatusNotFound, request(e, "/ping?test=1"))

	s, code := requestBody(e, "/metrics")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, s, `http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http"} 1`)
}

func TestDefaultRegistryMetrics(t *testing.T) {
	e := echo.New()

	prom := New(MiddlewareConfig{Namespace: "myapp"})
	e.Use(prom.Middleware())
	e.GET("/metrics", prom.ExporterHandler())

	assert.Equal(t, http.StatusNotFound, request(e, "/ping?test=1"))

	s, code := requestBody(e, "/metrics")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, s, `myapp_http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http"} 1`)
	unregisterDefaults("myapp")
}

func TestPrometheus_Buckets(t *testing.T) {
	e := echo.New()

	customRegistry := prometheus.NewRegistry()

	prom := New(MiddlewareConfig{Registry: customRegistry})
	e.Use(prom.Middleware())
	e.GET("/metrics", prom.ExporterHandler())

	assert.Equal(t, http.StatusNotFound, request(e, "/ping"))

	body, code := requestBody(e, "/metrics")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, `http_server_request_duration_seconds_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="0.005"}`, "duration should have time bucket (like, 0.005s)")
	assert.NotContains(t, body, `http_server_request_duration_seconds_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="512000"}`, "duration should NOT have a size bucket (like, 512K)")
	assert.Contains(t, body, `http_server_request_body_size_bytes_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="10240"}`, "request size should have a 1024k (size) bucket")
	assert.NotContains(t, body, `http_server_request_body_size_bytes_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="0.005"}`, "request size should NOT have time bucket (like, 0.005s)")
	assert.Contains(t, body, `http_server_response_body_size_bytes_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="10240"}`, "response size should have a 1024k (size) bucket")
	assert.NotContains(t, body, `echo_response_body_size_bytes_bucket{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http",le="0.005"}`, "response size should NOT have time bucket (like, 0.005s)")
}

func TestMiddlewareConfig_Skipper(t *testing.T) {
	e := echo.New()

	customRegistry := prometheus.NewRegistry()

	prom := New(MiddlewareConfig{
		Registry: customRegistry,
		Skipper: func(c echo.Context) bool {
			hasSuffix := strings.HasSuffix(c.Path(), "ignore")
			return hasSuffix
		},
	})
	e.Use(prom.Middleware())
	e.GET("/metrics", prom.ExporterHandler())

	e.GET("/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})
	e.GET("/test_ignore", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	assert.Equal(t, http.StatusNotFound, request(e, "/ping"))
	assert.Equal(t, http.StatusOK, request(e, "/test"))
	assert.Equal(t, http.StatusOK, request(e, "/test_ignore"))

	body, code := requestBody(e, "/metrics")
	assert.Equal(t, http.StatusOK, code)

	// 200 1
	assert.Contains(t, body, `http_server_request_body_size_bytes_bucket{http_request_method="GET",http_response_status_code="200",http_route="/test",url_scheme="http",le="1024"} 1`)
	// 404 1
	assert.Contains(t, body, `http_server_request_body_size_bytes_count{http_request_method="GET",http_response_status_code="404",http_route="",url_scheme="http"} 1`)
	assert.NotContains(t, body, `test_ignore`) // because we skipped
}

func TestMetricsForErrors(t *testing.T) {
	e := echo.New()
	customRegistry := prometheus.NewRegistry()
	prom := New(MiddlewareConfig{
		Namespace: "myapp",
		Registry:  customRegistry,
		Skipper: func(c echo.Context) bool {
			hasSuffix := strings.HasSuffix(c.Path(), "ignore")
			return hasSuffix
		},
	})
	e.Use(prom.Middleware())
	e.GET("/metrics", prom.ExporterHandler())

	e.GET("/handler_for_ok", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "OK")
	})
	e.GET("/handler_for_nok", func(c echo.Context) error {
		return c.JSON(http.StatusConflict, "NOK")
	})
	e.GET("/handler_for_error", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusBadGateway, "BAD")
	})

	assert.Equal(t, http.StatusOK, request(e, "/handler_for_ok"))
	assert.Equal(t, http.StatusConflict, request(e, "/handler_for_nok"))
	assert.Equal(t, http.StatusConflict, request(e, "/handler_for_nok"))
	assert.Equal(t, http.StatusBadGateway, request(e, "/handler_for_error"))

	body, code := requestBody(e, "/metrics")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, fmt.Sprintf("%s_requests_total", "myapp"))
	assert.Contains(t, body, `myapp_requests_total{http_request_method="GET",http_response_status_code="200",http_route="/handler_for_ok",url_scheme="http"} 1`)
	assert.Contains(t, body, `myapp_requests_total{http_request_method="GET",http_response_status_code="409",http_route="/handler_for_nok",url_scheme="http"} 2`)
	assert.Contains(t, body, `myapp_requests_total{http_request_method="GET",http_response_status_code="502",http_route="/handler_for_error",url_scheme="http"} 1`)
}

func requestBody(e *echo.Echo, path string) (string, int) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	return rec.Body.String(), rec.Code
}

func request(e *echo.Echo, path string) int {
	_, code := requestBody(e, path)
	return code
}

func unregisterDefaults(subsystem string) {
	// this is extremely hacky way to unregister our middleware metrics that it registers to prometheus default registry
	// Metrics/collector can be unregistered only by their instance but we do not have their instance, so we need to
	// create similar collector to register it and get error back with that existing collector we actually want to
	// unregister
	p := prometheus.DefaultRegisterer

	unRegisterCollector := func(opts prometheus.Opts) {
		dummyDuplicate := prometheus.NewCounterVec(prometheus.CounterOpts(opts), []string{"http_request_method", "http_response_status_code", "http_route", "server_address", "url_scheme"})
		err := p.Register(dummyDuplicate)
		if err == nil {
			return
		}
		// slog.Error("unregistering", "opts", opts, "err", err)
		var arErr prometheus.AlreadyRegisteredError
		if errors.As(err, &arErr) {
			p.Unregister(arErr.ExistingCollector)
		}
	}

	unRegisterCollector(prometheus.Opts{
		Subsystem: subsystem,
		Name:      "requests_total",
		Help:      "How many HTTP requests processed, partitioned by status code and HTTP method.",
	})
	unRegisterCollector(prometheus.Opts{
		Subsystem: subsystem,
		Name:      "http_server_active_requests",
		Help:      "Number of active HTTP server requests.",
	})
	unRegisterCollector(prometheus.Opts{
		Subsystem: subsystem,
		Name:      "http_server_request_duration_seconds",
		Help:      "Duration of HTTP server requests in seconds.",
	})
	unRegisterCollector(prometheus.Opts{
		Subsystem: subsystem,
		Name:      "http_server_response_body_size_bytes",
		Help:      "Size of HTTP server response bodies.",
	})
	unRegisterCollector(prometheus.Opts{
		Subsystem: subsystem,
		Name:      "http_server_request_body_size_bytes",
		Help:      "Size of HTTP server request bodies.",
	})
}

// WriteGatheredMetrics gathers collected metrics and writes them to given writer
func WriteGatheredMetrics(writer io.Writer, gatherer prometheus.Gatherer) error {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		return err
	}
	for _, mf := range metricFamilies {
		if _, err := expfmt.MetricFamilyToText(writer, mf); err != nil {
			return err
		}
	}
	return nil
}
