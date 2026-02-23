//go:build !windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// cmdInteractive 交互模式（REPL）。
func cmdInteractive(ctx context.Context, socketPath string, timeout time.Duration) error {
	client := NewClient(socketPath, timeout)

	// 测试连接
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("无法连接到调试服务: %w", err)
	}

	fmt.Println("xdbgctl 交互模式")
	fmt.Println("输入 'help' 查看可用命令，'quit' 或 'exit' 退出")
	fmt.Println()

	return runREPL(ctx, client)
}

// startInputReader 启动输入读取 goroutine。
// 设计决策: inputCh 无缓冲，使用 select 保护发送，
// 防止 context 取消后 goroutine 在 inputCh 发送端永久阻塞。
func startInputReader(ctx context.Context) (<-chan string, <-chan error) {
	inputCh := make(chan string)
	errCh := make(chan error, 1) // 缓冲区为 1，避免读取 goroutine 在 context 取消后泄漏

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			select {
			case inputCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
		close(inputCh)
	}()

	return inputCh, errCh
}

// runREPL 运行 REPL 循环。
// 使用 goroutine + channel 实现可取消的输入读取，确保 Ctrl+C 能立即退出。
func runREPL(ctx context.Context, exec executor) error {
	inputCh, errCh := startInputReader(ctx)

	for {
		fmt.Print("xdbg> ")

		select {
		case <-ctx.Done():
			fmt.Println("\n再见!")
			return nil
		case err := <-errCh:
			return fmt.Errorf("读取输入错误: %w", err)
		case line, ok := <-inputCh:
			if !ok {
				// EOF，正常退出
				fmt.Println()
				return nil
			}
			line = strings.TrimSpace(line)
			if shouldExit := processLine(ctx, exec, line); shouldExit {
				return nil
			}
		}
	}
}

// processLine 处理单行输入，返回 true 表示应该退出。
func processLine(ctx context.Context, exec executor, line string) bool {
	if line == "" {
		return false
	}

	// 检查退出命令
	if line == "quit" || line == "exit" {
		fmt.Println("再见!")
		return true
	}

	// 解析命令和参数
	parts := parseCommandLine(line)
	if len(parts) == 0 {
		return false
	}

	executeAndPrint(ctx, exec, parts[0], parts[1:])
	return false
}

// executeAndPrint 执行命令并打印结果。
func executeAndPrint(ctx context.Context, exec executor, command string, args []string) {
	resp, err := exec.Execute(ctx, command, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return
	}

	if !resp.Success {
		fmt.Fprintf(os.Stderr, "错误: %s\n", resp.Error)
		return
	}

	if resp.Output != "" {
		fmt.Println(resp.Output)
	}

	if resp.Truncated {
		fmt.Fprintf(os.Stderr, "[警告: 输出已截断，原始大小: %d 字节]\n", resp.OriginalSize)
	}

	fmt.Println()
}

// parseCommandLine 解析命令行，支持引号和反斜杠转义。
func parseCommandLine(line string) []string {
	var parts []string
	var current strings.Builder
	var inQuote bool
	var quoteChar rune
	var escaped bool

	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		switch {
		case isQuoteStart(r, inQuote):
			inQuote = true
			quoteChar = r
		case isQuoteEnd(r, quoteChar, inQuote):
			inQuote = false
			quoteChar = 0
		case isWordSeparator(r, inQuote):
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func isQuoteStart(r rune, inQuote bool) bool {
	return (r == '"' || r == '\'') && !inQuote
}

func isQuoteEnd(r, quoteChar rune, inQuote bool) bool {
	return r == quoteChar && inQuote
}

// 设计决策: 仅空格作为分词符，Tab 不分词。
// 交互式终端中 Tab 通常被解释为补全，不作为参数分隔。
// 管道输入场景（echo -e "cmd\targ" | xdbgctl interactive）Tab 会被视为参数的一部分。
func isWordSeparator(r rune, inQuote bool) bool {
	return r == ' ' && !inQuote
}
