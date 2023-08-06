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
	"github.com/ttys3/echo-otel-metrics/v2"
)

var serviceName = "otelmetric-demo"

func main() {
	rand.Seed(time.Now().UnixNano())
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	prom := echootelmetrics.New(echootelmetrics.MiddlewareConfig{
		Subsystem:      serviceName,
		Skipper:        URLSkipper,
		ServiceVersion: "v0.1.0",
	})
	prom.Setup(e)

	// Route => handler
	e.GET("/", func(c echo.Context) error {
		// Increment the counter.
		demoCounter.Add(c.Request().Context(), 1, metric.WithAttributes(attribute.String("foo", "bar")))
		demoCounter.Add(c.Request().Context(), 10, metric.WithAttributes(attribute.String("hello", "world")))
		demoCounter.Add(c.Request().Context(), 2, metric.WithAttributes(attribute.String("foo", "bar"), attribute.String("hello", "world")))
		time.Sleep(time.Millisecond * time.Duration(rand.Int63n(510)))
		return c.String(http.StatusOK, strings.Repeat("Hello, World!\n", rand.Intn(1024*500)/len("Hello, World!\n")))
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
