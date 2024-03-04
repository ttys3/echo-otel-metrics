package main

import (
	"go.opentelemetry.io/otel/metric"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/ttys3/echo-otel-metrics"
)

var serviceName = "otelmetric-demo"

var durationArray = []time.Duration{
	time.Millisecond * time.Duration(rand.Int63n(4)),
	time.Millisecond * time.Duration(rand.Int63n(8)),
	time.Millisecond * time.Duration(rand.Int63n(20)),
	time.Millisecond * time.Duration(rand.Int63n(60)),
	time.Millisecond * time.Duration(rand.Int63n(80)),
	time.Millisecond * time.Duration(rand.Int63n(200)),
	time.Millisecond * time.Duration(rand.Int63n(400)),
	time.Millisecond * time.Duration(rand.Int63n(600)),
	time.Millisecond * time.Duration(rand.Int63n(800)),
	time.Millisecond * time.Duration(rand.Int63n(1200)),
	time.Millisecond * time.Duration(rand.Int63n(2600)),
}

func main() {
	rand.Seed(time.Now().UnixNano())
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	prom := echootelmetrics.NewPrometheus(echootelmetrics.MiddlewareConfig{ServiceName: serviceName, ServiceVersion: "v0.1.0", Skipper: URLSkipper})
	prom.Setup(e)

	// Route => handler
	e.GET("/", func(c echo.Context) error {
		// Increment the counter.
		demoCounter.Add(c.Request().Context(), 1, metric.WithAttributes(attribute.String("foo", "bar")))
		demoCounter.Add(c.Request().Context(), 10, metric.WithAttributes(attribute.String("hello", "world")))
		demoCounter.Add(c.Request().Context(), 2, metric.WithAttributes(attribute.String("foo", "bar"), attribute.String("hello", "world")))
		time.Sleep(durationArray[rand.Int63n(int64(len(durationArray)-1))])
		return c.String(http.StatusOK, strings.Repeat("Hello, World!\n", rand.Intn(1024*5)/len("Hello, World!\n")))
	})

	// Start server
	log.Printf("view metrics at http://localhost:1323/metrics")
	log.Printf("serve at http://localhost:1323/")
	e.Logger.Fatal(e.Start(":1323"))
}

func URLSkipper(c echo.Context) bool {
	// skip /metrics
	if c.Request().RequestURI == "/metrics" {
		return true
	}

	return false
}
