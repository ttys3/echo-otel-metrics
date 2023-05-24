# echo-otel-metrics

this is an opentelemetry metrics middleware for echo http framework.

it addd a custom opentelemetry metrics middleware `otelmetric` to echo framework, and setup a prometheus exporter endpoint at `/metrics`.

this `otelmetric` middleware can work as a replacement for `github.com/labstack/echo-contrib/prometheus`

as of this module get v0.3.0 (otel metric v1.16.0), the metrics component is stable now, so this middleware is stable too.

but the metrics name diffs.

**counter**

```
requests_total_ratio_total
```


**histogram**

```
request_duration_milliseconds_bucket
request_duration_milliseconds_sum
request_duration_milliseconds_count

request_size_bytes_bucket
request_size_bytes_sum
request_size_bytes_count

response_size_bytes_bucket
response_size_bytes_sum
response_size_bytes_count
```

echo's middleware result:

```
request_duration_seconds_bucket{code="200",host="example.com",method="GET",url="/",le="0.005"}
```

but this one result:

```
request_duration_milliseconds_bucket{code="200",method="GET",otel_scope_name="echo",otel_scope_version="",url="/",le="5"}
```

and otel metric exporter also add an `target_info` **gauge** looks like:

```
target_info{service_name="otelmetric-demo",telemetry_sdk_language="go",telemetry_sdk_name="opentelemetry",telemetry_sdk_version="1.16.0"} 1
```

------------------------------------

The `echoprometheus` middleware registers the following metrics by default:

* Counter `requests_total`
* Histogram `request_duration_seconds`
* Histogram `response_size_bytes`
* Histogram `request_size_bytes`

------------------------------------

## warning

status https://opentelemetry.io/docs/instrumentation/go/

the Metrics component for OpenTelemetry Go is **Stable** now!

see https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.16.0

## docs

the implementation ref to https://github.com/labstack/echo-contrib/blob/master/prometheus/prometheus.go

https://uptrace.dev/opentelemetry/go-metrics.html

https://uptrace.dev/opentelemetry/prometheus-metrics.html#sending-go-metrics-to-prometheus

https://opentelemetry.io/docs/reference/specification/metrics/sdk/

https://echo.labstack.com/cookbook/hello-world/

semconv

https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/resource/semantic_conventions/README.md#document-conventions
https://github.com/open-telemetry/opentelemetry-go/blob/main/semconv/v1.12.0/resource.go
