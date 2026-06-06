package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	service, err := NewBridgeService(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Start(ctx); err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	Enabled                  bool     `json:"enabled"`
	BotToken                 string   `json:"botToken"`
	BridgeID                 string   `json:"bridgeId"`
	Host                     string   `json:"host"`
	Port                     int      `json:"port"`
	StorageRoot              string   `json:"storageRoot"`
	LogsRoot                 string   `json:"logsRoot"`
	DownloadsRoot            string   `json:"downloadsRoot"`
	AuditPath                string   `json:"auditPath"`
	StatePath                string   `json:"statePath"`
	AckReaction              string   `json:"ackReaction"`
	StatusReactions          []string `json:"statusReactions"`
	FinalReactionChoices     []string `json:"finalReactionChoices"`
	DefaultGuildID           string   `json:"defaultGuildId,omitempty"`
	AssignableChannelIDs     []string `json:"assignableChannelIds,omitempty"`
	AutoStartEnabledChannels bool     `json:"autoStartEnabledChannels,omitempty"`
	AutoStartAgentPrefix     string   `json:"autoStartAgentPrefix,omitempty"`
	OpenClawConfigPath       string   `json:"openClawConfigPath,omitempty"`
	DryRun                   bool     `json:"dryRun"`
}

func LoadConfig() (Config, error) {
	root := expandPath(envOr("DISCORD_BRIDGE_STORAGE_ROOT", "~/.pi/discord-bridge"))
	cfg := Config{
		Enabled:                  true,
		BotToken:                 strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		BridgeID:                 envOr("DISCORD_BRIDGE_ID", "discord-bridge-main"),
		Host:                     envOr("DISCORD_BRIDGE_HOST", "127.0.0.1"),
		Port:                     envOrInt("DISCORD_BRIDGE_PORT", 19444),
		StorageRoot:              root,
		LogsRoot:                 filepath.Join(root, "logs"),
		DownloadsRoot:            filepath.Join(root, "downloads"),
		AuditPath:                filepath.Join(root, "audit.jsonl"),
		StatePath:                filepath.Join(root, "state.json"),
		AckReaction:              envOr("DISCORD_BRIDGE_ACK_REACTION", "✅"),
		StatusReactions:          []string{"⏳", "🤖", "💭", "✅", "⚠️"},
		FinalReactionChoices:     []string{"✅", "👍", "👀", "🧠", "❤️"},
		DefaultGuildID:           strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_DEFAULT_GUILD_ID")),
		AssignableChannelIDs:     splitCSV(os.Getenv("DISCORD_BRIDGE_ASSIGNABLE_CHANNEL_IDS")),
		AutoStartEnabledChannels: envOrBool("DISCORD_BRIDGE_AUTOSTART_ENABLED_CHANNELS", false),
		AutoStartAgentPrefix:     envOr("DISCORD_BRIDGE_AUTOSTART_AGENT_PREFIX", "room"),
		OpenClawConfigPath:       expandPath(envOr("DISCORD_BRIDGE_OPENCLAW_CONFIG", "~/.openclaw/openclaw.json")),
		DryRun:                   envOrBool("DISCORD_BRIDGE_DRY_RUN", false),
	}
	if path := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CONFIG")); path != "" {
		raw, err := os.ReadFile(expandPath(path))
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}
	cfg.StorageRoot = expandPath(cfg.StorageRoot)
	cfg.LogsRoot = expandPath(cfg.LogsRoot)
	cfg.DownloadsRoot = expandPath(cfg.DownloadsRoot)
	cfg.AuditPath = expandPath(cfg.AuditPath)
	cfg.StatePath = expandPath(cfg.StatePath)
	cfg.OpenClawConfigPath = expandPath(cfg.OpenClawConfigPath)
	cfg.applyAutoAssignmentDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !c.Enabled {
		return errors.New("bridge must remain enabled for this prototype")
	}
	if c.BotToken == "" {
		return errors.New("DISCORD_BOT_TOKEN is required")
	}
	if c.BridgeID == "" || c.Host == "" || c.StorageRoot == "" {
		return errors.New("bridgeId, host, and storageRoot are required")
	}
	if c.Port <= 0 {
		return errors.New("port must be > 0")
	}
	if c.AckReaction == "" || len(c.StatusReactions) == 0 || len(c.FinalReactionChoices) == 0 {
		return errors.New("reaction config must not be empty")
	}
	if c.AutoStartEnabledChannels && strings.TrimSpace(c.AutoStartAgentPrefix) == "" {
		return errors.New("autoStartAgentPrefix must not be empty when autoStartEnabledChannels is enabled")
	}
	return nil
}

func (c *Config) applyAutoAssignmentDefaults() {
	if c.DefaultGuildID != "" && len(c.AssignableChannelIDs) > 0 {
		return
	}
	type channelEntry struct {
		Enabled bool `json:"enabled"`
	}
	type guildEntry struct {
		Channels map[string]channelEntry `json:"channels"`
	}
	type discordCfg struct {
		Guilds map[string]guildEntry `json:"guilds"`
	}
	type openClawCfg struct {
		Channels struct {
			Discord discordCfg `json:"discord"`
		} `json:"channels"`
	}
	raw, err := os.ReadFile(c.OpenClawConfigPath)
	if err != nil {
		return
	}
	var oc openClawCfg
	if json.Unmarshal(raw, &oc) != nil {
		return
	}
	if c.DefaultGuildID == "" {
		for guildID := range oc.Channels.Discord.Guilds {
			c.DefaultGuildID = guildID
			break
		}
	}
	if len(c.AssignableChannelIDs) == 0 && c.DefaultGuildID != "" {
		for channelID, entry := range oc.Channels.Discord.Guilds[c.DefaultGuildID].Channels {
			if entry.Enabled {
				c.AssignableChannelIDs = append(c.AssignableChannelIDs, channelID)
			}
		}
	}
}

type Envelope struct {
	BridgeID    string    `json:"bridgeId"`
	StartedAt   time.Time `json:"startedAt"`
	Host        string    `json:"host"`
	BotUserID   string    `json:"botUserId,omitempty"`
	BotTag      string    `json:"botTag,omitempty"`
	Transport   string    `json:"transport"`
	Version     string    `json:"version"`
	StorageRoot string    `json:"storageRoot"`
}

type Binding struct {
	AgentID   string    `json:"agentId"`
	GuildID   string    `json:"guildId"`
	ChannelID string    `json:"channelId"`
	JoinedAt  time.Time `json:"joinedAt"`
	Active    bool      `json:"active"`
}

type ManagedAgent struct {
	AgentID             string    `json:"agentId"`
	CredsRef            string    `json:"credsRef,omitempty"`
	DesiredState        string    `json:"desiredState,omitempty"`
	Command             string    `json:"command,omitempty"`
	Args                []string  `json:"args,omitempty"`
	WorkingDir          string    `json:"workingDir,omitempty"`
	RequestedGuildID    string    `json:"requestedGuildId,omitempty"`
	RequestedChannelID  string    `json:"requestedChannelId,omitempty"`
	AutoLaunch          bool      `json:"autoLaunch,omitempty"`
	LastJoinAt          time.Time `json:"lastJoinAt,omitempty"`
	LastQueuedAt        time.Time `json:"lastQueuedAt,omitempty"`
	LastCompletionAt    time.Time `json:"lastCompletionAt,omitempty"`
	LastLaunchedAt      time.Time `json:"lastLaunchedAt,omitempty"`
	LastStoppedAt       time.Time `json:"lastStoppedAt,omitempty"`
	LastProcessPID      int       `json:"lastProcessPid,omitempty"`
	LastObservedProcess string    `json:"lastObservedProcess,omitempty"`
	LastError           string    `json:"lastError,omitempty"`
}

type PersistedState struct {
	Bindings            map[string]Binding      `json:"bindings"`
	ProcessedMessageIDs map[string]int64        `json:"processedMessageIds"`
	ManagedReactions    map[string]string       `json:"managedReactions"`
	ManagedAgents       map[string]ManagedAgent `json:"managedAgents,omitempty"`
}

type AttachmentRecord struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	LocalPath   string `json:"localPath,omitempty"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType,omitempty"`
}

type InboundEvent struct {
	EventID     string             `json:"eventId"`
	MessageID   string             `json:"messageId"`
	AgentID     string             `json:"agentId"`
	GuildID     string             `json:"guildId,omitempty"`
	ChannelID   string             `json:"channelId"`
	AuthorID    string             `json:"authorId"`
	AuthorName  string             `json:"authorName"`
	Content     string             `json:"content"`
	Timestamp   time.Time          `json:"timestamp"`
	Attachments []AttachmentRecord `json:"attachments,omitempty"`
	ReplyToID   string             `json:"replyToId,omitempty"`
}

type LaunchedAgent struct {
	AgentID    string    `json:"agentId"`
	GuildID    string    `json:"guildId,omitempty"`
	ChannelID  string    `json:"channelId,omitempty"`
	Command    string    `json:"command"`
	Args       []string  `json:"args,omitempty"`
	WorkingDir string    `json:"workingDir,omitempty"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"startedAt"`
	LogPath    string    `json:"logPath"`
	State      string    `json:"state"`
}

type BridgeService struct {
	cfg            Config
	envelope       Envelope
	dg             *discordgo.Session
	httpServer     *http.Server
	started        time.Time
	state          PersistedState
	queues         map[string][]InboundEvent
	launchedAgents map[string]LaunchedAgent
	mu             sync.Mutex
}

func NewBridgeService(cfg Config) (*BridgeService, error) {
	for _, dir := range []string{cfg.StorageRoot, cfg.LogsRoot, cfg.DownloadsRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	s := &BridgeService{
		cfg:            cfg,
		started:        time.Now().UTC(),
		state:          state,
		queues:         map[string][]InboundEvent{},
		launchedAgents: map[string]LaunchedAgent{},
		envelope: Envelope{
			BridgeID:    cfg.BridgeID,
			StartedAt:   time.Now().UTC(),
			Host:        cfg.Host,
			Transport:   "discord",
			Version:     "v0",
			StorageRoot: cfg.StorageRoot,
		},
	}
	return s, nil
}

func (s *BridgeService) Start(ctx context.Context) error {
	dg, err := discordgo.New("Bot " + s.cfg.BotToken)
	if err != nil {
		return fmt.Errorf("discord session: %w", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent | discordgo.IntentsGuildMessageReactions
	dg.AddHandler(s.onReady)
	dg.AddHandler(s.onMessageCreate)
	s.dg = dg
	if !s.cfg.DryRun {
		if err := dg.Open(); err != nil {
			return fmt.Errorf("open discord session: %w", err)
		}
	}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpServer = &http.Server{Addr: fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port), Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		if s.dg != nil {
			s.dg.Close()
		}
	}()
	log.Printf("discord bridge listening on http://%s:%d dry_run=%v", s.cfg.Host, s.cfg.Port, s.cfg.DryRun)
	s.startAutoManagedAgents()
	if err := s.appendAudit("bridge.start", map[string]any{"bridgeId": s.cfg.BridgeID, "startedAt": s.started}); err != nil {
		return err
	}
	err = s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *BridgeService) onReady(_ *discordgo.Session, r *discordgo.Ready) {
	s.mu.Lock()
	s.envelope.BotUserID = r.User.ID
	s.envelope.BotTag = r.User.String()
	s.mu.Unlock()
	_ = s.appendAudit("discord.ready", map[string]any{"botUserId": r.User.ID, "botTag": r.User.String()})
}

func (s *BridgeService) onMessageCreate(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	s.mu.Lock()
	if _, seen := s.state.ProcessedMessageIDs[m.ID]; seen {
		s.mu.Unlock()
		return
	}
	s.state.ProcessedMessageIDs[m.ID] = time.Now().Unix()
	_ = s.saveStateLocked()
	binding, hasBinding := s.bindingForChannelLocked(m.ChannelID)
	s.mu.Unlock()

	attachments := s.persistAttachments(m.Message)
	_ = s.appendChatLog(m.Message, attachments, hasBinding)

	if !hasBinding {
		return
	}
	_ = s.setManagedReaction(m.ChannelID, m.ID, s.cfg.AckReaction)
	if len(s.cfg.StatusReactions) > 2 {
		_ = s.setManagedReaction(m.ChannelID, m.ID, s.cfg.StatusReactions[2])
	}
	event := InboundEvent{
		EventID:     fmt.Sprintf("evt_%s", m.ID),
		MessageID:   m.ID,
		AgentID:     binding.AgentID,
		GuildID:     m.GuildID,
		ChannelID:   m.ChannelID,
		AuthorID:    m.Author.ID,
		AuthorName:  m.Author.Username,
		Content:     m.Content,
		Timestamp:   m.Timestamp,
		Attachments: attachments,
	}
	if m.MessageReference != nil {
		event.ReplyToID = m.MessageReference.MessageID
	}
	s.mu.Lock()
	s.queues[binding.AgentID] = append(s.queues[binding.AgentID], event)
	managed := s.upsertManagedAgentLocked(ManagedAgent{AgentID: binding.AgentID, DesiredState: "running"})
	managed.LastQueuedAt = time.Now().UTC()
	s.state.ManagedAgents[binding.AgentID] = managed
	_ = s.saveStateLocked()
	s.mu.Unlock()
	_ = s.appendAudit("message.queued", event)
}

func (s *BridgeService) bindingForChannelLocked(channelID string) (Binding, bool) {
	for _, b := range s.state.Bindings {
		if b.Active && b.ChannelID == channelID {
			return b, true
		}
	}
	return Binding{}, false
}

func loadState(path string) (PersistedState, error) {
	state := PersistedState{
		Bindings:            map[string]Binding{},
		ProcessedMessageIDs: map[string]int64{},
		ManagedReactions:    map[string]string{},
		ManagedAgents:       map[string]ManagedAgent{},
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("parse state: %w", err)
	}
	if state.Bindings == nil {
		state.Bindings = map[string]Binding{}
	}
	if state.ProcessedMessageIDs == nil {
		state.ProcessedMessageIDs = map[string]int64{}
	}
	if state.ManagedReactions == nil {
		state.ManagedReactions = map[string]string{}
	}
	if state.ManagedAgents == nil {
		state.ManagedAgents = map[string]ManagedAgent{}
	}
	return state, nil
}

func (s *BridgeService) saveStateLocked() error {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.cfg.StatePath, raw, 0o644)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func envOrBool(key string, fallback bool) bool {
	if value := strings.TrimSpace(strings.ToLower(os.Getenv(key))); value != "" {
		return value == "1" || value == "true" || value == "yes" || value == "on"
	}
	return fallback
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
