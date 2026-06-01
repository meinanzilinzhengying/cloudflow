package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Config struct {
	timeout    time.Duration
	quiet      bool
	addresses  []string
	command    []string
}

func main() {
	var (
		timeoutStr string
		quiet      bool
	)

	flag.StringVar(&timeoutStr, "t", "30s", "Timeout for waiting (e.g., 30s, 1m)")
	flag.BoolVar(&quiet, "q", false, "Quiet mode")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid timeout: %v\n", err)
		printUsage()
		os.Exit(1)
	}

	// 拆分参数：地址列表 + 命令
	addresses := []string{}
	command := []string{}
	isCommand := false

	for i, arg := range args {
		if arg == "--" {
			isCommand = true
			continue
		}
		if isCommand {
			command = append(command, arg)
		} else {
			addresses = append(addresses, arg)
		}
	}

	if len(addresses) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one address (host:port) is required")
		printUsage()
		os.Exit(1)
	}

	cfg := &Config{
		timeout:    timeout,
		quiet:      quiet,
		addresses:  addresses,
		command:    command,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	if err := waitForServices(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if len(cfg.command) > 0 {
		if err := runCommand(cfg.command); err != nil {
			fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Println(`
wait-for - Wait for services to be available
Usage: wait-for [-t timeout] [-q] host:port [host:port...] [-- command...]
Examples:
  wait-for mysql:3306 redis:6379 -- ./start-app.sh
  wait-for -t 60s -q postgres:5432 -- echo "DB is ready"
`)
}

func waitForServices(ctx context.Context, cfg *Config) error {
	if !cfg.quiet {
		log.Printf("Waiting for %d service(s) (timeout: %v)...", len(cfg.addresses), cfg.timeout)
		for _, addr := range cfg.addresses {
			log.Printf("  - %s", addr)
		}
	}

	// 并行检查所有服务
	for _, addr := range cfg.addresses {
		if err := waitForAddress(ctx, addr, cfg.quiet); err != nil {
			return fmt.Errorf("service %s not available: %w", addr, err)
		}
	}

	if !cfg.quiet {
		log.Println("All services are available!")
	}
	return nil
}

func waitForAddress(ctx context.Context, address string, quiet bool) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", address, 1*time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

func runCommand(cmdline []string) error {
	if len(cmdline) == 0 {
		return nil
	}
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	// 等待命令退出
	err := cmd.Wait()
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			os.Exit(status.ExitStatus())
		}
		os.Exit(1)
	}
	return err
}
