package xdbg

import (
	"sort"
	"sync"
)

// essentialCommands 必要命令，始终允许执行。
// 这些命令对于基本的调试服务使用是必需的，不应被白名单阻止。
var essentialCommands = map[string]struct{}{
	"help": {},
	"exit": {},
}

// CommandRegistry 命令注册表。
type CommandRegistry struct {
	mu        sync.RWMutex
	commands  map[string]Command
	whitelist map[string]struct{} // nil 表示不启用白名单
}

// NewCommandRegistry 创建命令注册表。
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]Command),
	}
}

// Register 注册命令。
// 如果命令名已存在，将覆盖原有命令。
func (r *CommandRegistry) Register(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.Name()] = cmd
}

// Unregister 取消注册命令。
func (r *CommandRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.commands, name)
}

// Get 获取命令。
// 如果命令不存在，返回 nil。
func (r *CommandRegistry) Get(name string) Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.commands[name]
}

// Has 检查命令是否存在。
func (r *CommandRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.commands[name]
	return ok
}

// List 返回所有已注册的命令名（按字母排序）。
func (r *CommandRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Commands 返回所有已注册的命令（按名称排序）。
func (r *CommandRegistry) Commands() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmds := make([]Command, 0, len(r.commands))
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmds = append(cmds, r.commands[name])
	}
	return cmds
}

// SetWhitelist 设置命令白名单。
// 如果 whitelist 为 nil 或空，则禁用白名单（允许所有命令）。
func (r *CommandRegistry) SetWhitelist(whitelist []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(whitelist) == 0 {
		r.whitelist = nil
		return
	}

	r.whitelist = make(map[string]struct{}, len(whitelist))
	for _, name := range whitelist {
		r.whitelist[name] = struct{}{}
	}
}

// IsAllowed 检查命令是否被允许执行。
// 必要命令（help, exit）始终允许。
// 如果未设置白名单，返回 true。
func (r *CommandRegistry) IsAllowed(name string) bool {
	// 必要命令始终允许
	if _, ok := essentialCommands[name]; ok {
		return true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.whitelist == nil {
		return true
	}
	_, ok := r.whitelist[name]
	return ok
}

// Count 返回已注册命令的数量。
func (r *CommandRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.commands)
}
