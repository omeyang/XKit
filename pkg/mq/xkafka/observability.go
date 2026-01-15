package xkafka

import "github.com/omeyang/xkit/pkg/observability/xmetrics"

const (
	componentName = "xkafka"
)

func kafkaAttrs(topic string) []xmetrics.Attr {
	attrs := []xmetrics.Attr{xmetrics.String("messaging.system", "kafka")}
	if topic != "" {
		attrs = append(attrs, xmetrics.String("messaging.destination", topic))
	}
	return attrs
}
