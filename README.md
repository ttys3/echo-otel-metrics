# echo-otel-metrics

this is an opentelemetry metrics middleware for echo http framework.

it addd a custom opentelemetry metrics middleware `otelmetric` to echo framework, and setup a prometheus exporter endpoint at `/metrics`.

this `otelmetric` middleware can work as a replacement for

`https://github.com/labstack/echo-contrib/tree/master/echoprometheus`

or the legacy 
`https://github.com/labstack/echo-contrib/tree/master/prometheus`

as of this module get v0.3.0 (otel metric v1.16.0), the metrics component is stable now, so this middleware is stable too.

but the metrics name diffs.

**counter**

```
requests_ratio_total

compatible mode: requests_total
```


**histogram**

```
request_duration_milliseconds{_bucket, _sum, _count}

compatible mode: request_duration_seconds{_bucket, _sum, _count}

request_size_bytes{_bucket, _sum, _count}

response_size_bytes{_bucket, _sum, _count}
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

histogram will have `_bucket`, `_sum`, `_count` suffix.
------------------------------------

## warning

status https://opentelemetry.io/docs/instrumentation/go/

the Metrics component for OpenTelemetry Go is **Stable** now!

see https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.16.0

## prometheus related

### Unit `1`

https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/compatibility/prometheus_and_openmetrics.md#metric-metadata-1

> The Unit of an OTLP metric point SHOULD be converted to the equivalent unit in Prometheus when possible. This includes:
> * Special case: Converting "1" to "ratio".

https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/10554
Metrics about offset should use an empty unit.
Unit `1` must be used only for fractions and ratios.

https://github.com/open-telemetry/opentelemetry-specification/blob/ce360a26894271fa9a2b0c846a78d97d277e4183/specification/metrics/semantic_conventions/README.md#instrument-units

> Instrument Units
> 
> Units should follow the
> [Unified Code for Units of Measure](http://unitsofmeasure.org/ucum.html).
> 
> - Instruments for **utilization** metrics (that measure the fraction out of a
>   total) are dimensionless and SHOULD use the default unit `1` (the unity).
> - All non-units that use curly braces to annotate a quantity need to match the
>   grammatical number of the quantity it represent. For example if measuring the
>   number of individual requests to a process the unit would be `{request}`, not
>   `{requests}`.
> - Instruments that measure an integer count of something SHOULD only use
>   [annotations](https://ucum.org/ucum.html#para-curly) with curly braces to
>   give additional meaning *without* the leading default unit (`1`). For example,
>   use `{packet}`, `{error}`, `{fault}`, etc.
> - Instrument units other than `1` and those that use
>   [annotations](https://ucum.org/ucum.html#para-curly) SHOULD be specified using
>   the UCUM case sensitive ("c/s") variant.
>   For example, "Cel" for the unit with full name "degree Celsius".
> - Instruments SHOULD use non-prefixed units (i.e. `By` instead of `MiBy`)
>   unless there is good technical reason to not do so.
> - When instruments are measuring durations, seconds (i.e. `s`) SHOULD be used.
>

https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/8f394d4ffea8d5f06a9019245746ff253be106fd/pkg/translator/prometheus/normalize_name.go#L155-L162

Metric name normalization

https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/8f394d4ffea8d5f06a9019245746ff253be106fd/pkg/translator/prometheus/README.md#full-normalization

https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/translator/prometheus/README.md#full-normalization

## docs

the implementation ref to https://github.com/labstack/echo-contrib/blob/master/prometheus/prometheus.go

https://uptrace.dev/opentelemetry/go-metrics.html

https://uptrace.dev/opentelemetry/prometheus-metrics.html#sending-go-metrics-to-prometheus

https://opentelemetry.io/docs/reference/specification/metrics/sdk/

https://echo.labstack.com/cookbook/hello-world/

semconv

https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/resource/semantic_conventions/README.md#document-conventions
https://github.com/open-telemetry/opentelemetry-go/blob/main/semconv/v1.12.0/resource.go
