package xpulsar

import "github.com/omeyang/xkit/pkg/observability/xmetrics"

const componentName = "xpulsar"

func pulsarAttrs(topic string) []xmetrics.Attr {
	attrs := []xmetrics.Attr{xmetrics.String("messaging.system", "pulsar")}
	if topic != "" {
		attrs = append(attrs, xmetrics.String("messaging.destination.name", topic))
	}
	return attrs
}
