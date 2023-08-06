# echo-otel-metrics

this is an opentelemetry metrics middleware for echo http framework.

it addd a custom opentelemetry metrics middleware `echootelmetrics` to echo framework, and setup a prometheus exporter endpoint at `/metrics`.

from v2 version, this middleware follows the `Semantic Conventions for HTTP Metrics` spec.

and the metrics names are NOT compatible with echo official prometheus middleware any more.

- [HTTP Server](#http-server)
    * [Metric: `http.server.request.duration`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpserverrequestduration)
    * [Metric: `http.server.active_requests`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpserveractive_requests)
    * [Metric: `http.server.request.size`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpserverrequestsize)
    * [Metric: `http.server.response.size`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpserverresponsesize)
- [HTTP Client](#http-client)
    * [Metric: `http.client.request.duration`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpclientrequestduration)
    * [Metric: `http.client.request.size`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpclientrequestsize)
    * [Metric: `http.client.response.size`](https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md#metric-httpclientresponsesize)

https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-metrics.md

## usage

```go
import (
    "github.com/ttys3/echo-otel-metrics/v2"
)

	prom := echootelmetrics.New(echootelmetrics.MiddlewareConfig{
		Subsystem:      serviceName,
		Skipper:        URLSkipper,
		ServiceVersion: "v0.1.0",
	})
	prom.Setup(e)
```

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
