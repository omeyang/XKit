package xdbg

import (
	"context"
	"testing"
)

// mockCommand 测试用的 mock 命令。
type mockCommand struct {
	name string
	help string
}

func (c *mockCommand) Name() string {
	return c.name
}

func (c *mockCommand) Help() string {
	return c.help
}

func (c *mockCommand) Execute(_ context.Context, _ []string) (string, error) {
	return "mock output", nil
}

func TestCommandRegistry_Register(t *testing.T) {
	registry := NewCommandRegistry()

	cmd := &mockCommand{name: "test", help: "test command"}
	registry.Register(cmd)

	if !registry.Has("test") {
		t.Error("expected command 'test' to be registered")
	}

	got := registry.Get("test")
	if got != cmd {
		t.Error("expected to get the same command instance")
	}
}

func TestCommandRegistry_Unregister(t *testing.T) {
	registry := NewCommandRegistry()

	cmd := &mockCommand{name: "test", help: "test command"}
	registry.Register(cmd)
	registry.Unregister("test")

	if registry.Has("test") {
		t.Error("expected command 'test' to be unregistered")
	}
}

func TestCommandRegistry_Get_NotFound(t *testing.T) {
	registry := NewCommandRegistry()

	got := registry.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent command")
	}
}

func TestCommandRegistry_List(t *testing.T) {
	registry := NewCommandRegistry()

	registry.Register(&mockCommand{name: "zebra", help: "z"})
	registry.Register(&mockCommand{name: "alpha", help: "a"})
	registry.Register(&mockCommand{name: "beta", help: "b"})

	list := registry.List()

	// 应该按字母排序
	expected := []string{"alpha", "beta", "zebra"}
	if len(list) != len(expected) {
		t.Fatalf("List() length = %d, want %d", len(list), len(expected))
	}

	for i, name := range expected {
		if list[i] != name {
			t.Errorf("List()[%d] = %q, want %q", i, list[i], name)
		}
	}
}

func TestCommandRegistry_Commands(t *testing.T) {
	registry := NewCommandRegistry()

	registry.Register(&mockCommand{name: "zebra", help: "z"})
	registry.Register(&mockCommand{name: "alpha", help: "a"})

	cmds := registry.Commands()

	if len(cmds) != 2 {
		t.Fatalf("Commands() length = %d, want 2", len(cmds))
	}

	// 应该按字母排序
	if cmds[0].Name() != "alpha" {
		t.Errorf("Commands()[0].Name() = %q, want %q", cmds[0].Name(), "alpha")
	}
	if cmds[1].Name() != "zebra" {
		t.Errorf("Commands()[1].Name() = %q, want %q", cmds[1].Name(), "zebra")
	}
}

func TestCommandRegistry_Count(t *testing.T) {
	registry := NewCommandRegistry()

	if registry.Count() != 0 {
		t.Errorf("Count() = %d, want 0", registry.Count())
	}

	registry.Register(&mockCommand{name: "a", help: "a"})
	registry.Register(&mockCommand{name: "b", help: "b"})

	if registry.Count() != 2 {
		t.Errorf("Count() = %d, want 2", registry.Count())
	}
}

func TestCommandRegistry_Whitelist(t *testing.T) {
	registry := NewCommandRegistry()

	registry.Register(&mockCommand{name: "help", help: "h"})
	registry.Register(&mockCommand{name: "exit", help: "e"})
	registry.Register(&mockCommand{name: "setlog", help: "s"})

	// 默认无白名单，所有命令都允许
	if !registry.IsAllowed("help") {
		t.Error("expected 'help' to be allowed without whitelist")
	}
	if !registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be allowed without whitelist")
	}

	// 设置白名单
	registry.SetWhitelist([]string{"help", "exit"})

	if !registry.IsAllowed("help") {
		t.Error("expected 'help' to be allowed with whitelist")
	}
	if !registry.IsAllowed("exit") {
		t.Error("expected 'exit' to be allowed with whitelist")
	}
	if registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be forbidden with whitelist")
	}

	// 清空白名单
	registry.SetWhitelist(nil)

	if !registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be allowed after clearing whitelist")
	}
}

func TestCommandRegistry_OverwriteCommand(t *testing.T) {
	registry := NewCommandRegistry()

	cmd1 := &mockCommand{name: "test", help: "first"}
	cmd2 := &mockCommand{name: "test", help: "second"}

	registry.Register(cmd1)
	registry.Register(cmd2)

	got := registry.Get("test")
	if got.Help() != "second" {
		t.Errorf("expected command to be overwritten, got help = %q", got.Help())
	}
}

func TestCommandFunc(t *testing.T) {
	executed := false
	cmd := NewCommandFunc("test", "test command", func(_ context.Context, args []string) (string, error) {
		executed = true
		return "output: " + args[0], nil
	})

	if cmd.Name() != "test" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "test")
	}

	if cmd.Help() != "test command" {
		t.Errorf("Help() = %q, want %q", cmd.Help(), "test command")
	}

	output, err := cmd.Execute(context.Background(), []string{"arg1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !executed {
		t.Error("expected function to be executed")
	}

	if output != "output: arg1" {
		t.Errorf("Execute() output = %q, want %q", output, "output: arg1")
	}
}

func TestCommandRegistry_EssentialCommandsAlwaysAllowed(t *testing.T) {
	registry := NewCommandRegistry()

	registry.Register(&mockCommand{name: "help", help: "h"})
	registry.Register(&mockCommand{name: "exit", help: "e"})
	registry.Register(&mockCommand{name: "setlog", help: "s"})
	registry.Register(&mockCommand{name: "stack", help: "st"})

	// 设置白名单，只允许 setlog
	registry.SetWhitelist([]string{"setlog"})

	// 白名单中的命令应该被允许
	if !registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be allowed (in whitelist)")
	}

	// 必要命令应该始终被允许，即使不在白名单中
	if !registry.IsAllowed("help") {
		t.Error("expected 'help' to be allowed (essential command)")
	}
	if !registry.IsAllowed("exit") {
		t.Error("expected 'exit' to be allowed (essential command)")
	}

	// 非必要命令不在白名单中应该被禁止
	if registry.IsAllowed("stack") {
		t.Error("expected 'stack' to be forbidden (not in whitelist)")
	}
}

func TestCommandRegistry_EssentialCommandsWithEmptyWhitelist(t *testing.T) {
	registry := NewCommandRegistry()

	registry.Register(&mockCommand{name: "help", help: "h"})
	registry.Register(&mockCommand{name: "exit", help: "e"})
	registry.Register(&mockCommand{name: "setlog", help: "s"})

	// 设置空白名单（仅允许必要命令）
	registry.SetWhitelist([]string{})

	// 必要命令应该仍然被允许
	if !registry.IsAllowed("help") {
		t.Error("expected 'help' to be allowed even with empty whitelist")
	}
	if !registry.IsAllowed("exit") {
		t.Error("expected 'exit' to be allowed even with empty whitelist")
	}

	// 非必要命令应该被禁止
	if registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be forbidden with empty whitelist")
	}
}

func TestCommandRegistry_NilVsEmptyWhitelist(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&mockCommand{name: "setlog", help: "s"})

	// nil 白名单: 允许所有
	registry.SetWhitelist(nil)
	if !registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be allowed with nil whitelist")
	}

	// 空切片白名单: 仅必要命令
	registry.SetWhitelist([]string{})
	if registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be forbidden with empty whitelist")
	}

	// 恢复 nil
	registry.SetWhitelist(nil)
	if !registry.IsAllowed("setlog") {
		t.Error("expected 'setlog' to be allowed after restoring nil whitelist")
	}
}
