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
	Enabled              bool     `json:"enabled"`
	BotToken             string   `json:"botToken"`
	BridgeID             string   `json:"bridgeId"`
	Host                 string   `json:"host"`
	Port                 int      `json:"port"`
	StorageRoot          string   `json:"storageRoot"`
	LogsRoot             string   `json:"logsRoot"`
	DownloadsRoot        string   `json:"downloadsRoot"`
	AuditPath            string   `json:"auditPath"`
	StatePath            string   `json:"statePath"`
	AckReaction          string   `json:"ackReaction"`
	StatusReactions      []string `json:"statusReactions"`
	FinalReactionChoices []string `json:"finalReactionChoices"`
	DryRun               bool     `json:"dryRun"`
}

func LoadConfig() (Config, error) {
	root := expandPath(envOr("DISCORD_BRIDGE_STORAGE_ROOT", "~/.pi/discord-bridge"))
	cfg := Config{
		Enabled:              true,
		BotToken:             strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		BridgeID:             envOr("DISCORD_BRIDGE_ID", "discord-bridge-main"),
		Host:                 envOr("DISCORD_BRIDGE_HOST", "127.0.0.1"),
		Port:                 envOrInt("DISCORD_BRIDGE_PORT", 19444),
		StorageRoot:          root,
		LogsRoot:             filepath.Join(root, "logs"),
		DownloadsRoot:        filepath.Join(root, "downloads"),
		AuditPath:            filepath.Join(root, "audit.jsonl"),
		StatePath:            filepath.Join(root, "state.json"),
		AckReaction:          envOr("DISCORD_BRIDGE_ACK_REACTION", "✅"),
		StatusReactions:      []string{"⏳", "🤖", "💭", "✅", "⚠️"},
		FinalReactionChoices: []string{"✅", "👍", "👀", "🧠", "❤️"},
		DryRun:               envOrBool("DISCORD_BRIDGE_DRY_RUN", false),
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
	return nil
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

type PersistedState struct {
	Bindings            map[string]Binding `json:"bindings"`
	ProcessedMessageIDs map[string]int64   `json:"processedMessageIds"`
	ManagedReactions    map[string]string  `json:"managedReactions"`
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

type BridgeService struct {
	cfg        Config
	envelope   Envelope
	dg         *discordgo.Session
	httpServer *http.Server
	started    time.Time
	state      PersistedState
	queues     map[string][]InboundEvent
	mu         sync.Mutex
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
		cfg:     cfg,
		started: time.Now().UTC(),
		state:   state,
		queues:  map[string][]InboundEvent{},
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
