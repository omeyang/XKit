package xctx

import "context"

type contextFieldSetter struct {
	value string
	set   func(context.Context, string) (context.Context, error)
}

func applyOptionalFields(ctx context.Context, fields []contextFieldSetter) (context.Context, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		var err error
		ctx, err = field.set(ctx, field.value)
		if err != nil {
			return nil, err
		}
	}
	return ctx, nil
}
