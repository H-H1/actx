// Command ACTX is the main entry point for the ACTX CLI.
// ACTX 命令是 ACTX CLI 的主入口点。
package main

import (
	"fmt"
	"os"

	"github.com/H-H1/actx/internal/cli"
)

// main is the program entry point. It creates the root CLI command,
// executes it and exits with status 1 on error.
// main 是程序入口点。它创建根 CLI 命令，执行并在出错时以状态 1 退出。
func main() {
	rootCmd := cli.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
