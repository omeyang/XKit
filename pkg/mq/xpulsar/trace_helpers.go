package xpulsar

import (
	"context"

	"github.com/omeyang/xkit/internal/mqcore"

	"github.com/apache/pulsar-client-go/pulsar"
)

func injectPulsarTrace(ctx context.Context, tracer Tracer, msg *pulsar.ProducerMessage) {
	if tracer == nil || msg == nil {
		return
	}
	if msg.Properties == nil {
		msg.Properties = make(map[string]string)
	}
	tracer.Inject(ctx, msg.Properties)
}

func extractPulsarTrace(ctx context.Context, tracer Tracer, msg pulsar.Message) context.Context {
	if tracer == nil || msg == nil {
		return ctx
	}
	extracted := tracer.Extract(msg.Properties())
	return mqcore.MergeTraceContext(ctx, extracted)
}

func topicFromConsumerOptions(opts pulsar.ConsumerOptions) string {
	if opts.Topic != "" {
		return opts.Topic
	}
	if len(opts.Topics) == 1 {
		return opts.Topics[0]
	}
	if len(opts.Topics) > 1 {
		return "multi"
	}
	if opts.TopicsPattern != "" {
		return "pattern"
	}
	return ""
}
