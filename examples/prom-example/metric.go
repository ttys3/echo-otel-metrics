package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"strings"
)

var demoCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name:      "foobar",
	Help:      "Just a test counter",
	Subsystem: strings.ReplaceAll(serviceName, "-", "_"),
	Namespace: strings.ReplaceAll(serviceName, "-", "_"),
}, []string{"event", "domain"})
