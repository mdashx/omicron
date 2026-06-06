package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mdashx/omicron/research/agent-rpc/bridge"
	"github.com/mdashx/omicron/research/agent-rpc/harness"
)

func main() {
	cfg := harness.DefaultConfig()
	bridgeCfg := bridge.DefaultConfig()
	var message string
	var bridgeMode bool
	flag.StringVar(&cfg.Command, "command", cfg.Command, "upstream command")
	flag.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "working directory")
	flag.StringVar(&cfg.AgentID, "agent-id", cfg.AgentID, "logical agent id")
	flag.StringVar(&cfg.SessionDir, "session-dir", "", "optional Pi session dir")
	flag.BoolVar(&cfg.NoSession, "no-session", false, "disable Pi session persistence")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable debug logging to stderr")
	flag.StringVar(&message, "message", "", "one-shot prompt")
	flag.BoolVar(&bridgeMode, "bridge", false, "run in Discord bridge polling mode")
	flag.StringVar(&bridgeCfg.BridgeURL, "bridge-url", bridgeCfg.BridgeURL, "Discord bridge URL")
	flag.StringVar(&bridgeCfg.AgentID, "bridge-agent-id", bridgeCfg.AgentID, "Discord bridge agent id")
	flag.StringVar(&bridgeCfg.CredsRef, "bridge-creds-ref", bridgeCfg.CredsRef, "Discord bridge credentials reference")
	flag.StringVar(&bridgeCfg.GuildID, "bridge-guild-id", bridgeCfg.GuildID, "Discord bridge guild id")
	flag.StringVar(&bridgeCfg.ChannelID, "bridge-channel-id", bridgeCfg.ChannelID, "Discord bridge channel id")
	flag.StringVar(&bridgeCfg.StateRoot, "bridge-state-root", bridgeCfg.StateRoot, "local bridge runner state root")
	flag.Parse()
	cfg.Args = []string{"--mode", "rpc"}
	if cfg.NoSession {
		cfg.Args = append(cfg.Args, "--no-session")
	}
	if cfg.SessionDir != "" {
		cfg.Args = append(cfg.Args, "--session-dir", cfg.SessionDir)
	}
	if bridgeCfg.AgentID == "" {
		bridgeCfg.AgentID = cfg.AgentID
	}
	cfg.AgentID = bridgeCfg.AgentID
	cfg = cfg.Resolved()
	bridgeCfg = bridgeCfg.Resolved()
	h := harness.NewHarness(cfg)
	defer h.Close()
	if bridgeMode {
		runner, err := bridge.NewRunner(bridgeCfg, h)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bridge runner error:", err)
			os.Exit(1)
		}
		ctx := context.Background()
		if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "bridge run error:", err)
			os.Exit(1)
		}
		return
	}
	if err := h.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start error:", err)
		os.Exit(1)
	}
	state, err := h.GetState(context.Background())
	if err == nil {
		fmt.Fprintf(os.Stderr, "session: %s %s\n", state.SessionID, state.SessionFile)
	}
	if message != "" || flag.NArg() > 0 {
		if message == "" {
			message = strings.Join(flag.Args(), " ")
		}
		runPrompt(h, message)
		return
	}
	repl(h)
}

func repl(h *harness.Harness) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "read error:", err)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch line {
		case "/quit", "/exit":
			return
		case "/state":
			state, err := h.GetState(context.Background())
			if err != nil {
				fmt.Fprintln(os.Stderr, "state error:", err)
				continue
			}
			fmt.Printf("session=%s\nfile=%s\nstreaming=%v\n", state.SessionID, state.SessionFile, state.IsStreaming)
			continue
		default:
			runPrompt(h, line)
		}
	}
}

func runPrompt(h *harness.Harness, message string) {
	fmt.Fprintln(os.Stderr, "status: started")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := h.Prompt(ctx, message)
	if err != nil {
		fmt.Fprintln(os.Stderr, "status: error")
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stderr, "status: completed")
	fmt.Println(result.Text)
}
