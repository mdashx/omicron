package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

func (s *BridgeService) startAutoManagedAgents() {
	if !s.cfg.AutoStartEnabledChannels {
		return
	}
	go func() {
		// Let HTTP come up first so child bridge clients can join/poll successfully.
		time.Sleep(2 * time.Second)
		if err := s.bootstrapAutoManagedAgents(); err != nil {
			log.Printf("auto-managed-agent bootstrap error: %v", err)
		}
	}()
}

func (s *BridgeService) bootstrapAutoManagedAgents() error {
	managedIDs, err := s.ensureManagedAgentsForEnabledChannels()
	if err != nil {
		return err
	}
	for _, agentID := range managedIDs {
		if err := s.ensureManagedAgentRunning(agentID); err != nil {
			log.Printf("auto-managed-agent start failed agent=%s: %v", agentID, err)
			s.recordManagedAgentError(agentID, err)
		}
	}
	return nil
}

func (s *BridgeService) ensureManagedAgentsForEnabledChannels() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	managedByChannel := map[string]string{}
	for agentID, agent := range s.state.ManagedAgents {
		if channelID := strings.TrimSpace(agent.RequestedChannelID); channelID != "" {
			managedByChannel[channelID] = agentID
		}
	}
	createdOrExisting := make([]string, 0, len(s.cfg.AssignableChannelIDs))
	changed := false
	for _, channelID := range s.cfg.AssignableChannelIDs {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}
		if agentID, ok := managedByChannel[channelID]; ok {
			createdOrExisting = append(createdOrExisting, agentID)
			continue
		}
		agentID := fmt.Sprintf("%s-%s", strings.TrimSpace(s.cfg.AutoStartAgentPrefix), channelID)
		agent := s.upsertManagedAgentLocked(ManagedAgent{
			AgentID:            agentID,
			CredsRef:           "local-session",
			DesiredState:       "running",
			RequestedGuildID:   s.cfg.DefaultGuildID,
			RequestedChannelID: channelID,
			AutoLaunch:         true,
		})
		s.state.ManagedAgents[agentID] = agent
		createdOrExisting = append(createdOrExisting, agentID)
		changed = true
	}
	if changed {
		if err := s.saveStateLocked(); err != nil {
			return nil, err
		}
	}
	return createdOrExisting, nil
}

func (s *BridgeService) ensureManagedAgentRunning(agentID string) error {
	s.mu.Lock()
	agent, ok := s.state.ManagedAgents[agentID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("managed agent not found")
	}
	if strings.TrimSpace(agent.DesiredState) == "disabled" || strings.TrimSpace(agent.DesiredState) == "stopped" {
		s.mu.Unlock()
		return nil
	}
	if launched, ok := s.launchedAgents[agentID]; ok && launched.State == "running" {
		s.mu.Unlock()
		return nil
	}
	if binding, ok := s.state.Bindings[agentID]; ok && binding.Active {
		// Assume externally running / already joined for now; avoid duplicate spawn.
		s.mu.Unlock()
		return nil
	}
	command := agent.Command
	args := append([]string(nil), agent.Args...)
	workingDir := agent.WorkingDir
	guildID := firstNonEmpty(agent.RequestedGuildID, s.cfg.DefaultGuildID)
	channelID := agent.RequestedChannelID
	s.mu.Unlock()
	_, err := s.launchAgent(launchAgentRequest{
		AgentID:    agentID,
		GuildID:    guildID,
		ChannelID:  channelID,
		Command:    command,
		Args:       args,
		WorkingDir: workingDir,
	})
	return err
}
