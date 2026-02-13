package xctx

import "context"

type contextFieldSetter struct {
	value string
	set   func(context.Context, string) (context.Context, error)
}

// 设计决策: 仅注入非空字段（"跳过空值"语义），无法表达"显式清空/覆盖为空"。
// 这是有意选择：context 值本身不可变，清空语义需要调用方构建新的 context 链。
// 在中间件链进行"部分覆盖"时，父 context 中已存在的字段会被保留，
// 这是预期行为——允许入口层设置基础值，后续层仅补充缺失字段。
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
		// 设计决策: 此错误分支当前不可达（所有 setter 仅对 nil ctx 返回错误，
		// 而 nil 已在上方检查），但保留作为防御性编程，防止未来 setter 添加新的校验逻辑。
		if err != nil {
			return nil, err
		}
	}
	return ctx, nil
}
