#!/usr/bin/env bash

set -eou pipefail

go test -v -run=TestCompModeCustomRegistryMetricsDoNotRecord404Route
go test -v -run=TestDefaultRegistryMetrics
go test -v -run=TestPrometheus_Buckets
go test -v -run=TestMiddlewareConfig_Skipper
go test -v -run=TestMetricsForErrors

