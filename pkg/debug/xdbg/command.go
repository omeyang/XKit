package xdbg

import "context"

// Command 调试命令接口。
type Command interface {
	// Name 返回命令名称。
	Name() string

	// Help 返回命令帮助信息。
	Help() string

	// Execute 执行命令。
	// args 是命令参数（不包含命令名本身）。
	// 返回命令输出或错误。
	Execute(ctx context.Context, args []string) (string, error)
}

// CommandFunc 函数式命令实现。
type CommandFunc struct {
	name    string
	help    string
	execute func(ctx context.Context, args []string) (string, error)
}

// NewCommandFunc 创建函数式命令。
//
// 前置条件: name 不能为空，fn 不能为 nil。违反时 panic（编程错误，应在开发期发现）。
func NewCommandFunc(name, help string, fn func(ctx context.Context, args []string) (string, error)) *CommandFunc {
	if name == "" {
		panic("xdbg: NewCommandFunc: name must not be empty")
	}
	if fn == nil {
		panic("xdbg: NewCommandFunc: fn must not be nil")
	}
	return &CommandFunc{
		name:    name,
		help:    help,
		execute: fn,
	}
}

// Name 返回命令名称。
func (c *CommandFunc) Name() string {
	return c.name
}

// Help 返回命令帮助信息。
func (c *CommandFunc) Help() string {
	return c.help
}

// Execute 执行命令。
func (c *CommandFunc) Execute(ctx context.Context, args []string) (string, error) {
	return c.execute(ctx, args)
}
