package echootelmetrics

import "go.opentelemetry.io/otel/attribute"

const (
	// MetricHTTPServerRequestDuration http.server.request.duration https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#metric-httpserverrequestduration
	MetricHTTPServerRequestDuration = "http.server.request.duration"

	// MetricHTTPServerActiveRequests http.server.active_requests https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#metric-httpserveractive_requests
	MetricHTTPServerActiveRequests = "http.server.active_requests"

	// MetricHTTPServerRequestBodySize http.server.request.body.size https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#metric-httpserverrequestbodysize
	MetricHTTPServerRequestBodySize = "http.server.request.body.size"

	// MetricHTTPServerResponseBodySize http.server.response.body.size https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#metric-httpserverresponsebodysize
	MetricHTTPServerResponseBodySize = "http.server.response.body.size"
)

const (

	// NetworkProtocolName network.protocol.name
	NetworkProtocolName = attribute.Key("network.protocol.name")

	// NetworkProtocolVersion network.protocol.version
	NetworkProtocolVersion = attribute.Key("network.protocol.version")

	// ErrorType error.type
	ErrorType = attribute.Key("error.type")

	// URLScheme url.scheme
	URLScheme = attribute.Key("url.scheme")

	// ServerAddress server.address
	ServerAddress = attribute.Key("server.address")

	// ServerPort server.port
	ServerPort = attribute.Key("server.port")

	// HttpRoute http.route
	HttpRoute = attribute.Key("http.route")

	// HttpRequestMethod http.request.method
	HttpRequestMethod = attribute.Key("http.request.method")

	// HttpResponseStatusCode http.response.status_code
	HttpResponseStatusCode = attribute.Key("http.response.status_code")
)
