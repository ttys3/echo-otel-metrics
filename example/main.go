package main

import (
	"github.com/ttys3/echo-otel-metrics/otelmetric"
	"go.opentelemetry.io/otel/attribute"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var serviceName = "otelmetric-demo"

func main() {
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	prom := otelmetric.NewPrometheus(serviceName, URLSkipper)
	prom.Use(e)

	// Route => handler
	e.GET("/", func(c echo.Context) error {
		// Increment the counter.
		demoCounter.Add(c.Request().Context(), 1, attribute.String("foo", "bar"))
		demoCounter.Add(c.Request().Context(), 10, attribute.String("hello", "world"))
		return c.String(http.StatusOK, "Hello, World!\n")
	})

	// Start server
	e.Logger.Fatal(e.Start(":1323"))
}

func URLSkipper(c echo.Context) bool {

	// skip /metrics
	if c.Request().RequestURI == "/metrics" {
		return true
	}

	return false
}
