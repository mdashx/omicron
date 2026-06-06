package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (s *BridgeService) onInteractionCreate(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	if i == nil || i.Type != discordgo.InteractionMessageComponent || i.Member == nil && i.User == nil {
		return
	}
	data := i.MessageComponentData()
	if !strings.HasPrefix(data.CustomID, "model-picker:") {
		return
	}
	userID := interactionUserID(i)
	s.mu.Lock()
	pending, ok := s.pendingModelPickers[data.CustomID]
	if ok {
		delete(s.pendingModelPickers, data.CustomID)
	}
	s.mu.Unlock()
	if !ok {
		s.respondInteraction(i, true, "[discord-bridge]\nModel picker expired or not found.")
		return
	}
	if pending.UserID != userID {
		s.respondInteraction(i, true, "[discord-bridge]\nThis model picker belongs to another user.")
		return
	}
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		s.respondInteraction(i, true, "[discord-bridge]\nModel picker expired.")
		return
	}
	if len(data.Values) == 0 {
		s.respondInteraction(i, true, "[discord-bridge]\nNo model selected.")
		return
	}
	modelID := strings.TrimSpace(data.Values[0])
	if modelID == "" {
		s.respondInteraction(i, true, "[discord-bridge]\nNo model selected.")
		return
	}
	binding, err := s.activeBinding(pending.AgentID)
	if err != nil {
		s.respondInteraction(i, true, "[discord-bridge]\nTarget agent is not currently bound.")
		return
	}
	event := InboundEvent{
		EventID:     fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
		MessageID:   pending.ReplyToMessage,
		AgentID:     pending.AgentID,
		GuildID:     binding.GuildID,
		ChannelID:   binding.ChannelID,
		AuthorID:    userID,
		AuthorName:  interactionUserName(i),
		Content:     modelID,
		Kind:        "set_model",
		CommandName: "model",
		Timestamp:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.queues[pending.AgentID] = append(s.queues[pending.AgentID], event)
	_ = s.saveStateLocked()
	s.mu.Unlock()
	_ = s.appendAudit("bridge.model_picker.selected", map[string]any{"authorId": userID, "agentId": pending.AgentID, "modelId": modelID, "channelId": pending.ChannelID})
	s.respondInteraction(i, true, fmt.Sprintf("[discord-bridge]\nApplying model to %s:\n%s", pending.AgentID, modelID))
}

func (s *BridgeService) openModelPicker(m *discordgo.MessageCreate, agentID string) error {
	if len(s.cfg.AvailableModels) == 0 {
		return fmt.Errorf("no available models configured for picker")
	}
	customID := fmt.Sprintf("model-picker:%d", time.Now().UnixNano())
	options := make([]discordgo.SelectMenuOption, 0, len(s.cfg.AvailableModels))
	for _, model := range s.cfg.AvailableModels {
		label := model.ID
		if strings.TrimSpace(model.Alias) != "" {
			label = model.Alias
		}
		options = append(options, discordgo.SelectMenuOption{
			Label:       truncateDiscordLabel(label, 100),
			Value:       model.ID,
			Description: truncateDiscordLabel(model.ID, 100),
		})
	}
	msg := &discordgo.MessageSend{
		Content:   bridgeResponse("Model picker", fmt.Sprintf("Agent: %s\nChoose a model below.", agentID)),
		Reference: &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{CustomID: customID, Placeholder: "Select a model", Options: options},
			}},
		},
	}
	if _, err := s.dg.ChannelMessageSendComplex(m.ChannelID, msg); err != nil {
		return err
	}
	s.mu.Lock()
	s.pendingModelPickers[customID] = pendingModelPicker{UserID: m.Author.ID, AgentID: agentID, ChannelID: m.ChannelID, ReplyToMessage: m.ID, CreatedAt: time.Now().UTC()}
	s.mu.Unlock()
	_ = s.appendAudit("bridge.model_picker.opened", map[string]any{"authorId": m.Author.ID, "agentId": agentID, "channelId": m.ChannelID})
	return nil
}

func (s *BridgeService) respondInteraction(i *discordgo.InteractionCreate, ephemeral bool, content string) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	_ = s.dg.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: flags},
	})
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func interactionUserName(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username
	}
	if i.User != nil {
		return i.User.Username
	}
	return "unknown"
}

func truncateDiscordLabel(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 1 {
		return text[:max]
	}
	return text[:max-1] + "…"
}
