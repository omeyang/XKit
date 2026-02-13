package xconf

// options 定义配置加载选项。
type options struct {
	// delim 配置键的分隔符，默认为 "."。
	delim string

	// tag 结构体标签名，用于 Unmarshal，默认为 "koanf"。
	tag string
}

// Option 定义配置选项函数类型。
type Option func(*options)

// defaultOptions 返回默认配置选项。
func defaultOptions() *options {
	return &options{
		delim: ".",
		tag:   "koanf",
	}
}

// validate 校验配置选项。
func (o *options) validate() error {
	if o.delim == "" {
		return ErrInvalidDelim
	}
	if o.tag == "" {
		return ErrInvalidTag
	}
	return nil
}

// WithDelim 设置配置键分隔符。
// 默认为 "."，例如 "app.server.port"。
func WithDelim(delim string) Option {
	return func(o *options) {
		o.delim = delim
	}
}

// WithTag 设置结构体标签名。
// 默认为 "koanf"，用于 Unmarshal 时的字段映射。
func WithTag(tag string) Option {
	return func(o *options) {
		o.tag = tag
	}
}
