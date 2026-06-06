package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type channelOption struct {
	ID      string `json:"id"`
	GuildID string `json:"guildId,omitempty"`
	Name    string `json:"name"`
	Label   string `json:"label"`
}

type bindingView struct {
	AgentID        string
	GuildID        string
	ChannelID      string
	ChannelLabel   string
	State          string
	QueueDepth     int
	DesiredAgent   string
	DesiredChannel string
	LastJoinedAt   time.Time
	NeedsAttention bool
}

type activityItem struct {
	Timestamp time.Time
	Type      string
	Summary   string
}

type dashboardData struct {
	Overview           OverviewStats      `json:"overview"`
	ManagedAgents      []ManagedAgentView `json:"managedAgents"`
	Bindings           []bindingView      `json:"bindings"`
	Activity           []activityItem     `json:"activity"`
	AssignableChannels []channelOption    `json:"assignableChannels"`
	DefaultGuildID     string             `json:"defaultGuildId"`
	Envelope           Envelope           `json:"envelope"`
	DryRun             bool               `json:"dryRun"`
	Now                time.Time          `json:"now"`
}

type pageData struct {
	Title              string
	CurrentPath        string
	Overview           OverviewStats
	ManagedAgents      []ManagedAgentView
	Bindings           []bindingView
	Activity           []activityItem
	AssignableChannels []channelOption
	DefaultGuildID     string
	Envelope           Envelope
	DryRun             bool
	Now                time.Time
	Agent              *ManagedAgentView
	RecentLogs         []string
}

var uiTemplates = template.Must(template.New("base").Funcs(template.FuncMap{
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2006-01-02 15:04:05")
	},
	"since": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return time.Since(t).Round(time.Second).String() + " ago"
	},
	"join": strings.Join,
}).Parse(uiTemplateSource))

func (s *BridgeService) dashboardSnapshot() dashboardData {
	s.mu.Lock()
	envelope := s.envelope
	dryRun := s.cfg.DryRun
	defaultGuildID := s.cfg.DefaultGuildID
	assignableChannelIDs := append([]string(nil), s.cfg.AssignableChannelIDs...)
	s.mu.Unlock()
	return dashboardData{
		Overview:           s.overviewStats(),
		ManagedAgents:      s.managedAgentViews(),
		Bindings:           s.bindingViews(),
		Activity:           s.activityItems(30),
		AssignableChannels: s.resolveAssignableChannels(defaultGuildID, assignableChannelIDs),
		DefaultGuildID:     defaultGuildID,
		Envelope:           envelope,
		DryRun:             dryRun,
		Now:                time.Now().UTC(),
	}
}

func (s *BridgeService) pageData(title, currentPath string) pageData {
	snapshot := s.dashboardSnapshot()
	return pageData{
		Title:              title,
		CurrentPath:        currentPath,
		Overview:           snapshot.Overview,
		ManagedAgents:      snapshot.ManagedAgents,
		Bindings:           snapshot.Bindings,
		Activity:           snapshot.Activity,
		AssignableChannels: snapshot.AssignableChannels,
		DefaultGuildID:     snapshot.DefaultGuildID,
		Envelope:           snapshot.Envelope,
		DryRun:             snapshot.DryRun,
		Now:                snapshot.Now,
	}
}

func (s *BridgeService) handleHomePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := s.pageData("Discord Bridge Overview", "/")
	s.renderHTML(w, "page_overview", data)
}

func (s *BridgeService) handleManagedAgentsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleManagedAgentCreate(w, r)
		return
	}
	data := s.pageData("Managed Agents", "/managed-agents")
	s.renderHTML(w, "page_agents", data)
}

func (s *BridgeService) handleBindingsPage(w http.ResponseWriter, r *http.Request) {
	data := s.pageData("Bindings", "/bindings")
	s.renderHTML(w, "page_bindings", data)
}

func (s *BridgeService) handleActivityPage(w http.ResponseWriter, r *http.Request) {
	data := s.pageData("Activity", "/activity")
	s.renderHTML(w, "page_activity", data)
}

func (s *BridgeService) handleOverviewPartial(w http.ResponseWriter, _ *http.Request) {
	data := s.pageData("Discord Bridge Overview", "/")
	s.renderHTML(w, "partial_overview", data)
}

func (s *BridgeService) handleManagedAgentsTablePartial(w http.ResponseWriter, _ *http.Request) {
	data := s.pageData("Managed Agents", "/managed-agents")
	s.renderHTML(w, "partial_agents_table", data)
}

func (s *BridgeService) handleBindingsTablePartial(w http.ResponseWriter, _ *http.Request) {
	data := s.pageData("Bindings", "/bindings")
	s.renderHTML(w, "partial_bindings_table", data)
}

func (s *BridgeService) handleActivityFeedPartial(w http.ResponseWriter, _ *http.Request) {
	data := s.pageData("Activity", "/activity")
	s.renderHTML(w, "partial_activity_feed", data)
}

func (s *BridgeService) handleManagedAgentsUI(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/managed-agents/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	agentID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		view, ok := s.findManagedAgentView(agentID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		data := s.pageData("Managed Agent "+agentID, "/managed-agents")
		data.Agent = &view
		data.RecentLogs = readAgentLogSince(view.LogPath, maxTime(view.LastActivityAt.Add(-5*time.Minute), time.Now().Add(-1*time.Hour)), 40)
		s.renderHTML(w, "page_agent_detail", data)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	s.handleManagedAgentAction(w, r, agentID, parts[1])
}

func (s *BridgeService) handleManagedAgentCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agentID := strings.TrimSpace(r.FormValue("agentId"))
	if agentID == "" {
		http.Error(w, "agentId is required", http.StatusBadRequest)
		return
	}
	args := splitArgs(r.FormValue("args"))
	s.mu.Lock()
	agent := s.upsertManagedAgentLocked(ManagedAgent{
		AgentID:            agentID,
		CredsRef:           firstNonEmpty(strings.TrimSpace(r.FormValue("credsRef")), "local-session"),
		DesiredState:       firstNonEmpty(strings.TrimSpace(r.FormValue("desiredState")), "stopped"),
		Command:            strings.TrimSpace(r.FormValue("command")),
		Args:               args,
		WorkingDir:         strings.TrimSpace(r.FormValue("workingDir")),
		RequestedGuildID:   strings.TrimSpace(r.FormValue("guildId")),
		RequestedChannelID: strings.TrimSpace(r.FormValue("channelId")),
	})
	s.state.ManagedAgents[agentID] = agent
	err := s.saveStateLocked()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.appendAudit("managed_agent.upsert", map[string]any{"agentId": agentID})
	s.respondManagedAgentMutation(w, r)
}

func (s *BridgeService) handleManagedAgentAction(w http.ResponseWriter, r *http.Request, agentID, action string) {
	s.mu.Lock()
	agent, ok := s.state.ManagedAgents[agentID]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "managed agent not found", http.StatusNotFound)
		return
	}
	switch action {
	case "start":
		_, err := s.launchAgent(launchAgentRequest{
			AgentID:    agentID,
			GuildID:    agent.RequestedGuildID,
			ChannelID:  agent.RequestedChannelID,
			Command:    agent.Command,
			Args:       append([]string(nil), agent.Args...),
			WorkingDir: agent.WorkingDir,
		})
		if err != nil {
			s.recordManagedAgentError(agentID, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case "stop":
		if _, err := s.stopAgent(agentID); err != nil {
			s.recordManagedAgentError(agentID, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case "restart":
		_, _ = s.stopAgent(agentID)
		_, err := s.launchAgent(launchAgentRequest{
			AgentID:    agentID,
			GuildID:    agent.RequestedGuildID,
			ChannelID:  agent.RequestedChannelID,
			Command:    agent.Command,
			Args:       append([]string(nil), agent.Args...),
			WorkingDir: agent.WorkingDir,
		})
		if err != nil {
			s.recordManagedAgentError(agentID, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	s.respondManagedAgentMutation(w, r)
}

func (s *BridgeService) respondManagedAgentMutation(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/managed-agents", http.StatusSeeOther)
		return
	}
	data := s.pageData("Managed Agents", "/managed-agents")
	s.renderHTML(w, "partial_agents_table", data)
}

func (s *BridgeService) recordManagedAgentError(agentID string, err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	managed := s.upsertManagedAgentLocked(ManagedAgent{AgentID: agentID})
	managed.LastError = err.Error()
	s.state.ManagedAgents[agentID] = managed
	_ = s.saveStateLocked()
	s.mu.Unlock()
}

func (s *BridgeService) bindingViews() []bindingView {
	views := s.managedAgentViews()
	viewByID := map[string]ManagedAgentView{}
	for _, view := range views {
		viewByID[view.AgentID] = view
	}
	s.mu.Lock()
	bindings := make([]Binding, 0, len(s.state.Bindings))
	for _, binding := range s.state.Bindings {
		bindings = append(bindings, binding)
	}
	managed := make(map[string]ManagedAgent, len(s.state.ManagedAgents))
	for k, v := range s.state.ManagedAgents {
		managed[k] = v
	}
	assignableChannelIDs := append([]string(nil), s.cfg.AssignableChannelIDs...)
	defaultGuildID := s.cfg.DefaultGuildID
	s.mu.Unlock()
	channelOptions := s.resolveAssignableChannels(defaultGuildID, assignableChannelIDs)
	channelLabels := map[string]string{}
	for _, option := range channelOptions {
		channelLabels[option.ID] = option.Label
	}
	result := make([]bindingView, 0, len(bindings)+len(managed))
	seen := map[string]bool{}
	for _, binding := range bindings {
		view := viewByID[binding.AgentID]
		result = append(result, bindingView{
			AgentID:        binding.AgentID,
			GuildID:        binding.GuildID,
			ChannelID:      binding.ChannelID,
			ChannelLabel:   firstNonEmpty(channelLabels[binding.ChannelID], binding.ChannelID),
			State:          "active",
			QueueDepth:     view.QueueDepth,
			DesiredAgent:   binding.AgentID,
			DesiredChannel: view.ChannelID,
			LastJoinedAt:   binding.JoinedAt,
			NeedsAttention: view.NeedsAttention,
		})
		seen[binding.AgentID] = true
	}
	for agentID, agent := range managed {
		if seen[agentID] || agent.RequestedChannelID == "" {
			continue
		}
		view := viewByID[agentID]
		result = append(result, bindingView{
			AgentID:        agentID,
			GuildID:        agent.RequestedGuildID,
			ChannelID:      agent.RequestedChannelID,
			ChannelLabel:   firstNonEmpty(channelLabels[agent.RequestedChannelID], agent.RequestedChannelID),
			State:          "desired",
			QueueDepth:     view.QueueDepth,
			DesiredAgent:   agentID,
			DesiredChannel: agent.RequestedChannelID,
			LastJoinedAt:   agent.LastJoinAt,
			NeedsAttention: view.NeedsAttention,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChannelID < result[j].ChannelID })
	return result
}

func (s *BridgeService) activityItems(limit int) []activityItem {
	records := readAuditTail(s.cfg.AuditPath, limit)
	items := make([]activityItem, 0, len(records))
	for _, record := range records {
		items = append(items, activityItem{Timestamp: record.Timestamp, Type: record.Type, Summary: summarizeAuditRecord(record)})
	}
	return items
}

func summarizeAuditRecord(record auditRecord) string {
	payload, _ := json.Marshal(record.Payload)
	text := string(payload)
	if len(text) > 160 {
		text = text[:160] + "..."
	}
	return text
}

func (s *BridgeService) findManagedAgentView(agentID string) (ManagedAgentView, bool) {
	for _, view := range s.managedAgentViews() {
		if view.AgentID == agentID {
			return view, true
		}
	}
	return ManagedAgentView{}, false
}

func (s *BridgeService) renderHTML(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := uiTemplates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}

func (s *BridgeService) resolveAssignableChannels(guildID string, ids []string) []channelOption {
	options := make([]channelOption, 0, len(ids))
	for _, id := range ids {
		name := id
		label := id
		if s.dg != nil {
			if ch, err := s.dg.State.Channel(id); err == nil && ch != nil {
				if ch.Name != "" {
					name = ch.Name
				}
			} else if ch, err := s.dg.Channel(id); err == nil && ch != nil && ch.Name != "" {
				name = ch.Name
			}
		}
		if name != id {
			label = "#" + name + " (" + id + ")"
		}
		options = append(options, channelOption{ID: id, GuildID: guildID, Name: name, Label: label})
	}
	return options
}

func readAgentLogSince(path string, startedAt time.Time, limit int) []string {
	lines := readAllLines(path)
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if ts, ok := parseLogTimestamp(trimmed); ok {
			if !startedAt.IsZero() && ts.Before(startedAt.Add(-1*time.Second)) {
				continue
			}
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

func parseLogTimestamp(line string) (time.Time, bool) {
	if len(line) < len("2006/01/02 15:04:05") {
		return time.Time{}, false
	}
	ts, err := time.Parse("2006/01/02 15:04:05", line[:len("2006/01/02 15:04:05")])
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func readAuditTail(path string, limit int) []auditRecord {
	var records []auditRecord
	for _, line := range readLastLines(path, limit) {
		var rec auditRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			records = append(records, rec)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Timestamp.After(records[j].Timestamp) })
	if len(records) > limit {
		return records[:limit]
	}
	return records
}

func readAllLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines
}

func readLastLines(path string, limit int) []string {
	lines := readAllLines(path)
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	if len(trimmed) > limit {
		return trimmed[len(trimmed)-limit:]
	}
	return trimmed
}

func splitArgs(raw string) []string {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) == 0 {
		return nil
	}
	return parts
}

const uiTemplateSource = `
{{define "header"}}
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <script src="https://unpkg.com/htmx.org@1.9.12"></script>
  <style>
    :root { color-scheme: dark; }
    body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 0; background: #0b1020; color: #e5e7eb; }
    header { padding: 20px 24px; background: #11182d; border-bottom: 1px solid #24304d; }
    main { padding: 24px; display: grid; gap: 16px; }
    nav { display:flex; gap:12px; margin-top:12px; flex-wrap:wrap; }
    nav a { color:#cbd5e1; text-decoration:none; padding:8px 12px; border:1px solid #24304d; border-radius:999px; }
    nav a.active { background:#2563eb; border-color:#2563eb; color:white; }
    .grid { display:grid; gap:16px; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); }
    .grid-2 { display:grid; gap:16px; grid-template-columns: 1.2fr 0.8fr; }
    .card { background:#121a30; border:1px solid #24304d; border-radius:12px; padding:16px; overflow:auto; }
    h1,h2,h3,p { margin-top:0; }
    table { width:100%; border-collapse: collapse; }
    th,td { text-align:left; padding:8px; border-bottom:1px solid #24304d; vertical-align:top; }
    .muted { color:#9ca3af; }
    .badge { display:inline-block; padding:2px 8px; border-radius:999px; background:#1e293b; border:1px solid #334155; font-size:12px; }
    .good { background:#14532d; border-color:#166534; }
    .warn { background:#78350f; border-color:#92400e; }
    .bad { background:#7f1d1d; border-color:#991b1b; }
    form.inline { display:inline; }
    input, select, button { background:#0f172a; color:#e5e7eb; border:1px solid #334155; border-radius:8px; padding:8px; }
    button { cursor:pointer; }
    .actions { display:flex; gap:8px; flex-wrap:wrap; }
    .stack { display:grid; gap:10px; }
    pre { white-space:pre-wrap; word-break:break-word; }
    @media (max-width: 900px) { .grid-2 { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <header>
    <h1>Discord Bridge</h1>
    <div class="muted">Bridge ID {{.Envelope.BridgeID}} · Host {{.Envelope.Host}} · Dry run {{.DryRun}}</div>
    <nav>
      <a href="/" {{if eq .CurrentPath "/"}}class="active"{{end}}>Overview</a>
      <a href="/managed-agents" {{if eq .CurrentPath "/managed-agents"}}class="active"{{end}}>Managed Agents</a>
      <a href="/bindings" {{if eq .CurrentPath "/bindings"}}class="active"{{end}}>Bindings</a>
      <a href="/activity" {{if eq .CurrentPath "/activity"}}class="active"{{end}}>Activity</a>
    </nav>
  </header>
  <main>
{{end}}

{{define "footer"}}
  </main>
</body>
</html>
{{end}}

{{define "page_overview"}}{{template "header" .}}{{template "content_overview" .}}{{template "footer" .}}{{end}}
{{define "page_agents"}}{{template "header" .}}{{template "content_agents" .}}{{template "footer" .}}{{end}}
{{define "page_bindings"}}{{template "header" .}}{{template "content_bindings" .}}{{template "footer" .}}{{end}}
{{define "page_activity"}}{{template "header" .}}{{template "content_activity" .}}{{template "footer" .}}{{end}}
{{define "page_agent_detail"}}{{template "header" .}}{{template "content_agent_detail" .}}{{template "footer" .}}{{end}}

{{define "content_overview"}}
  <section class="card" id="overview" hx-get="/partials/overview" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_overview" .}}</section>
  <section class="grid-2">
    <div class="card" id="agents-table" hx-get="/partials/managed-agents-table" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_agents_table" .}}</div>
    <div class="card" id="activity-feed" hx-get="/partials/activity-feed" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_activity_feed" .}}</div>
  </section>
{{end}}

{{define "content_agents"}}
  <section class="grid-2">
    <div class="card" id="agents-table" hx-get="/partials/managed-agents-table" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_agents_table" .}}</div>
    <div class="card">
      <h2>Register or update managed agent</h2>
      <form class="stack" method="post" action="/managed-agents" hx-post="/managed-agents" hx-target="#agents-table" hx-swap="outerHTML">
        <label>Agent ID<br><input name="agentId" required></label>
        <label>Creds Ref<br><input name="credsRef" value="local-session"></label>
        <label>Desired State<br>
          <select name="desiredState">
            <option value="stopped">stopped</option>
            <option value="running">running</option>
            <option value="disabled">disabled</option>
          </select>
        </label>
        <label>Guild ID<br><input name="guildId" value="{{.DefaultGuildID}}"></label>
        <label>Channel<br>
          <select name="channelId">
            <option value="">Auto-assign</option>
            {{range .AssignableChannels}}<option value="{{.ID}}">{{.Label}}</option>{{end}}
          </select>
        </label>
        <label>Command<br><input name="command" value="go"></label>
        <label>Args<br><input name="args" value="run ./cmd/agent-rpc --bridge"></label>
        <label>Working Dir<br><input name="workingDir" value=""></label>
        <button type="submit">Save managed agent</button>
      </form>
    </div>
  </section>
{{end}}

{{define "content_bindings"}}
  <section class="card" id="bindings-table" hx-get="/partials/bindings-table" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_bindings_table" .}}</section>
{{end}}

{{define "content_activity"}}
  <section class="card" id="activity-feed" hx-get="/partials/activity-feed" hx-trigger="load, every 10s" hx-swap="outerHTML">{{template "partial_activity_feed" .}}</section>
{{end}}

{{define "content_agent_detail"}}
  <section class="card">
    <p><a href="/managed-agents">← Back to managed agents</a></p>
    {{if .Agent}}
      <h2>{{.Agent.AgentID}}</h2>
      <div class="grid">
        <div><strong>Desired</strong><br><span class="badge">{{.Agent.DesiredState}}</span></div>
        <div><strong>Process</strong><br><span class="badge">{{.Agent.ProcessState}}</span></div>
        <div><strong>Bridge</strong><br><span class="badge">{{.Agent.BridgeState}}</span></div>
        <div><strong>Work</strong><br><span class="badge">{{.Agent.WorkState}}</span></div>
      </div>
      <p class="muted">{{.Agent.CommandLabel}}</p>
      <p>Channel: <code>{{.Agent.ChannelID}}</code> · Guild: <code>{{.Agent.GuildID}}</code> · PID: <code>{{.Agent.PID}}</code></p>
      <p>Last activity: {{since .Agent.LastActivityAt}} · Last completion: {{since .Agent.LastCompletionAt}} · Last join: {{since .Agent.LastJoinAt}}</p>
      {{if .Agent.LastError}}<p><span class="badge bad">Last error</span> {{.Agent.LastError}}</p>{{end}}
      <div class="actions">
        <form class="inline" method="post" action="/managed-agents/{{.Agent.AgentID}}/start"><button>Start</button></form>
        <form class="inline" method="post" action="/managed-agents/{{.Agent.AgentID}}/stop"><button>Stop</button></form>
        <form class="inline" method="post" action="/managed-agents/{{.Agent.AgentID}}/restart"><button>Restart</button></form>
      </div>
      <h3>Recent logs</h3>
      <pre>{{join .RecentLogs "\n"}}</pre>
    {{end}}
  </section>
{{end}}

{{define "partial_overview"}}
<div class="card" id="overview">
  <h2>Overview</h2>
  <div class="grid">
    <div><strong>Discord</strong><br><span class="badge {{if eq .Overview.DiscordStatus "connected"}}good{{else}}bad{{end}}">{{.Overview.DiscordStatus}}</span></div>
    <div><strong>Uptime</strong><br>{{.Overview.BridgeUptime}}</div>
    <div><strong>Managed agents</strong><br>{{.Overview.ManagedAgents}}</div>
    <div><strong>Healthy joined</strong><br>{{.Overview.HealthyJoined}}</div>
    <div><strong>Queued events</strong><br>{{.Overview.QueuedEvents}}</div>
    <div><strong>Needs attention</strong><br>{{.Overview.NeedsAttention}}</div>
  </div>
</div>
{{end}}

{{define "partial_agents_table"}}
<div class="card" id="agents-table">
  <h2>Managed Agents</h2>
  <table>
    <thead><tr><th>Agent</th><th>Desired</th><th>Process</th><th>Bridge</th><th>Work</th><th>Binding</th><th>Queue</th><th>Last activity</th><th>Actions</th></tr></thead>
    <tbody>
      {{range .ManagedAgents}}
      <tr>
        <td><a href="/managed-agents/{{.AgentID}}">{{.AgentID}}</a>{{if .NeedsAttention}} <span class="badge warn">attention</span>{{end}}<div class="muted">{{.CommandLabel}}</div></td>
        <td><span class="badge">{{.DesiredState}}</span></td>
        <td><span class="badge {{if eq .ProcessState "running"}}good{{else if or (eq .ProcessState "failed") (eq .ProcessState "exited")}}bad{{else}}warn{{end}}">{{.ProcessState}}</span></td>
        <td><span class="badge {{if eq .BridgeState "bound"}}good{{else if eq .BridgeState "stale"}}bad{{else}}warn{{end}}">{{.BridgeState}}</span></td>
        <td><span class="badge">{{.WorkState}}</span></td>
        <td>{{if .ChannelID}}<code>{{.ChannelID}}</code>{{else}}<span class="muted">unassigned</span>{{end}}</td>
        <td>{{.QueueDepth}}</td>
        <td>{{since .LastActivityAt}}</td>
        <td>
          <div class="actions">
            {{if .CanStart}}<form class="inline" method="post" action="/managed-agents/{{.AgentID}}/start" hx-post="/managed-agents/{{.AgentID}}/start" hx-target="#agents-table" hx-swap="outerHTML"><button>Start</button></form>{{end}}
            {{if .CanStop}}<form class="inline" method="post" action="/managed-agents/{{.AgentID}}/stop" hx-post="/managed-agents/{{.AgentID}}/stop" hx-target="#agents-table" hx-swap="outerHTML"><button>Stop</button></form>{{end}}
            {{if .CanRestart}}<form class="inline" method="post" action="/managed-agents/{{.AgentID}}/restart" hx-post="/managed-agents/{{.AgentID}}/restart" hx-target="#agents-table" hx-swap="outerHTML"><button>Restart</button></form>{{end}}
          </div>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="9" class="muted">No managed agents yet.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{define "partial_bindings_table"}}
<div class="card" id="bindings-table">
  <h2>Bindings</h2>
  <table>
    <thead><tr><th>Agent</th><th>State</th><th>Guild</th><th>Channel</th><th>Queue</th><th>Last join</th></tr></thead>
    <tbody>
      {{range .Bindings}}
      <tr>
        <td><a href="/managed-agents/{{.AgentID}}">{{.AgentID}}</a>{{if .NeedsAttention}} <span class="badge warn">attention</span>{{end}}</td>
        <td><span class="badge">{{.State}}</span></td>
        <td><code>{{.GuildID}}</code></td>
        <td>{{if .ChannelLabel}}{{.ChannelLabel}}{{else}}<code>{{.ChannelID}}</code>{{end}}</td>
        <td>{{.QueueDepth}}</td>
        <td>{{since .LastJoinedAt}}</td>
      </tr>
      {{else}}
      <tr><td colspan="6" class="muted">No bindings yet.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{define "partial_activity_feed"}}
<div class="card" id="activity-feed">
  <h2>Activity</h2>
  <table>
    <thead><tr><th>Time</th><th>Type</th><th>Summary</th></tr></thead>
    <tbody>
      {{range .Activity}}
      <tr><td>{{since .Timestamp}}</td><td><code>{{.Type}}</code></td><td>{{.Summary}}</td></tr>
      {{else}}
      <tr><td colspan="3" class="muted">No recent activity.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
`
