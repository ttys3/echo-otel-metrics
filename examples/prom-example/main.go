package main

import (
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
	// required for pprof
	_ "net/http/pprof"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	e.Use(echoprometheus.NewMiddleware("myapp"))   // register middleware to gather metrics from requests
	e.GET("/metrics", echoprometheus.NewHandler()) // register route to serve gathered metrics in Prometheus format

	// Route => handler
	e.GET("/", func(c echo.Context) error {
		// Increment the counter.
		demoCounter.WithLabelValues("foo", "bar").Inc()
		demoCounter.WithLabelValues(c.Request().Header.Get("X-B3-Traceid"), "world").Add(10)
		demoCounter.WithLabelValues("foo", "bar").Add(2)
		time.Sleep(durationArray[rand.Int63n(int64(len(durationArray)-1))])
		return c.String(http.StatusOK, strings.Repeat("Hello, World!\n", rand.Intn(1024*5)/len("Hello, World!\n")))
	})

	e.GET("/memory-test", memoryTestHandler)

	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

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

func memoryTestHandler(c echo.Context) error {
	// Increment the counter.
	demoCounter.WithLabelValues("foo", "bar").Inc()
	demoCounter.WithLabelValues(strings.Repeat(c.Request().Header.Get("X-B3-Traceid"), 1024), "world").Add(10)
	demoCounter.WithLabelValues("foo", "bar").Add(2)
	return c.String(http.StatusOK, strings.Repeat("Hello, World!\n", rand.Intn(1024*5)/len("Hello, World!\n")))
}
