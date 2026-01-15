package xconf

// Options 定义配置加载选项。
type Options struct {
	// Delim 配置键的分隔符，默认为 "."。
	Delim string

	// Tag 结构体标签名，用于 Unmarshal，默认为 "koanf"。
	Tag string
}

// Option 定义配置选项函数类型。
type Option func(*Options)

// defaultOptions 返回默认配置选项。
func defaultOptions() *Options {
	return &Options{
		Delim: ".",
		Tag:   "koanf",
	}
}

// WithDelim 设置配置键分隔符。
// 默认为 "."，例如 "app.server.port"。
func WithDelim(delim string) Option {
	return func(o *Options) {
		o.Delim = delim
	}
}

// WithTag 设置结构体标签名。
// 默认为 "koanf"，用于 Unmarshal 时的字段映射。
func WithTag(tag string) Option {
	return func(o *Options) {
		o.Tag = tag
	}
}
