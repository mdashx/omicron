package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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

type dashboardData struct {
	Envelope                Envelope                 `json:"envelope"`
	Bindings                map[string]Binding       `json:"bindings"`
	QueueSizes              map[string]int           `json:"queueSizes"`
	DryRun                  bool                     `json:"dryRun"`
	RecentChats             []chatLogRecord          `json:"recentChats"`
	RecentAttachments       []AttachmentRecord       `json:"recentAttachments"`
	AttachmentCount         int                      `json:"attachmentCount"`
	AuditTail               []auditRecord            `json:"auditTail"`
	ManagedReactions        map[string]string        `json:"managedReactions"`
	StatusReactions         []string                 `json:"statusReactions"`
	FinalChoices            []string                 `json:"finalReactionChoices"`
	AssignableChannelIDs    []string                 `json:"assignableChannelIds"`
	AssignableChannels      []channelOption          `json:"assignableChannels"`
	DefaultGuildID          string                   `json:"defaultGuildId"`
	LaunchedAgents          map[string]LaunchedAgent `json:"launchedAgents"`
	LaunchedAgentLogs       map[string][]string      `json:"launchedAgentLogs"`
	LaunchedAgentPTYInputs  map[string][]string      `json:"launchedAgentPtyInputs"`
	LaunchedAgentPTYOutputs map[string][]string      `json:"launchedAgentPtyOutputs"`
	Now                     time.Time                `json:"now"`
}

func (s *BridgeService) dashboardSnapshot() dashboardData {
	s.mu.Lock()
	bindings := make(map[string]Binding, len(s.state.Bindings))
	for k, v := range s.state.Bindings {
		bindings[k] = v
	}
	queueSizes := make(map[string]int, len(s.queues))
	for k, v := range s.queues {
		queueSizes[k] = len(v)
	}
	reactions := make(map[string]string, len(s.state.ManagedReactions))
	for k, v := range s.state.ManagedReactions {
		reactions[k] = v
	}
	launchedAgents := make(map[string]LaunchedAgent, len(s.launchedAgents))
	for k, v := range s.launchedAgents {
		launchedAgents[k] = v
	}
	envelope := s.envelope
	dryRun := s.cfg.DryRun
	statusReactions := append([]string(nil), s.cfg.StatusReactions...)
	finalChoices := append([]string(nil), s.cfg.FinalReactionChoices...)
	assignableChannelIDs := append([]string(nil), s.cfg.AssignableChannelIDs...)
	defaultGuildID := s.cfg.DefaultGuildID
	logsRoot := s.cfg.LogsRoot
	downloadsRoot := s.cfg.DownloadsRoot
	auditPath := s.cfg.AuditPath
	s.mu.Unlock()

	assignableChannels := s.resolveAssignableChannels(defaultGuildID, assignableChannelIDs)
	launchedAgentLogs := readLaunchedAgentLogs(launchedAgents, 30)
	launchedAgentPTYInputs := readLaunchedAgentTranscript(launchedAgents, "pty-input.log", 40)
	launchedAgentPTYOutputs := readLaunchedAgentTranscript(launchedAgents, "pty-output.log", 60)
	recentChats := readRecentChats(logsRoot, 20)
	recentAttachments, attachmentCount := readRecentAttachments(downloadsRoot, 20)
	auditTail := readAuditTail(auditPath, 20)

	return dashboardData{
		Envelope:                envelope,
		Bindings:                bindings,
		QueueSizes:              queueSizes,
		DryRun:                  dryRun,
		RecentChats:             recentChats,
		RecentAttachments:       recentAttachments,
		AttachmentCount:         attachmentCount,
		AuditTail:               auditTail,
		ManagedReactions:        reactions,
		StatusReactions:         statusReactions,
		FinalChoices:            finalChoices,
		AssignableChannelIDs:    assignableChannelIDs,
		AssignableChannels:      assignableChannels,
		DefaultGuildID:          defaultGuildID,
		LaunchedAgents:          launchedAgents,
		LaunchedAgentLogs:       launchedAgentLogs,
		LaunchedAgentPTYInputs:  launchedAgentPTYInputs,
		LaunchedAgentPTYOutputs: launchedAgentPTYOutputs,
		Now:                     time.Now().UTC(),
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

func readLaunchedAgentLogs(agents map[string]LaunchedAgent, limit int) map[string][]string {
	logs := make(map[string][]string, len(agents))
	for agentID, agent := range agents {
		if agent.LogPath == "" {
			continue
		}
		logs[agentID] = readAgentLogSince(agent.LogPath, agent.StartedAt, limit)
	}
	return logs
}

func readLaunchedAgentTranscript(agents map[string]LaunchedAgent, name string, limit int) map[string][]string {
	logs := make(map[string][]string, len(agents))
	for agentID, agent := range agents {
		if agent.LogPath == "" {
			continue
		}
		path := filepath.Join(filepath.Dir(agent.LogPath), name)
		logs[agentID] = readLastLines(path, limit)
	}
	return logs
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
			if ts.Before(startedAt.Add(-1 * time.Second)) {
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

func readRecentChats(root string, limit int) []chatLogRecord {
	var records []chatLogRecord
	_ = filepath.Walk(filepath.Join(root, "chats"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		for _, line := range readLastLines(path, 10) {
			var rec chatLogRecord
			if err := json.Unmarshal([]byte(line), &rec); err == nil {
				records = append(records, rec)
			}
		}
		return nil
	})
	sort.Slice(records, func(i, j int) bool { return records[i].Timestamp.After(records[j].Timestamp) })
	if len(records) > limit {
		return records[:limit]
	}
	return records
}

func readRecentAttachments(root string, limit int) ([]AttachmentRecord, int) {
	var records []AttachmentRecord
	count := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		count++
		records = append(records, AttachmentRecord{
			Filename:  info.Name(),
			LocalPath: path,
			Size:      int(info.Size()),
		})
		return nil
	})
	sort.Slice(records, func(i, j int) bool { return records[i].LocalPath > records[j].LocalPath })
	if len(records) > limit {
		records = records[:limit]
	}
	return records, count
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

func renderDashboardHTML() string {
	return `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Discord Bridge Dashboard</title>
  <style>
    :root { color-scheme: dark; }
    body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 0; background: #0b1020; color: #e5e7eb; }
    header { padding: 20px 24px; background: #11182d; position: sticky; top: 0; }
    main { padding: 24px; display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); }
    .card { background: #121a30; border: 1px solid #24304d; border-radius: 12px; padding: 16px; overflow: auto; }
    h1,h2,h3 { margin: 0 0 12px 0; }
    pre { white-space: pre-wrap; word-break: break-word; margin: 0; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid #24304d; vertical-align: top; }
    code { background: #0f172a; padding: 2px 6px; border-radius: 6px; }
    .muted { color: #9ca3af; }
  </style>
</head>
<body>
  <header>
    <h1>Discord Bridge Dashboard</h1>
    <div class="muted">Accessible on your VPN by binding the service to a VPN/Tailscale host or <code>0.0.0.0</code>.</div>
  </header>
  <main>
    <section class="card"><h2>Bridge</h2><pre id="bridge"></pre></section>
    <section class="card"><h2>Launch Agent</h2>
      <form id="launch-form">
        <div><label>Agent ID<br><input name="agentId" value="agent-1" style="width:100%"></label></div>
        <div><label>Guild ID (optional)<br><input name="guildId" style="width:100%"></label></div>
        <div><label>Channel<br><select name="channelId" id="channel-select" style="width:100%"><option value="">Auto-assign</option></select></label></div>
        <div><label>Command (optional)<br><input name="command" value="discoagent" style="width:100%"></label></div>
        <div style="margin-top:10px"><button type="submit">Launch</button></div>
      </form>
      <div id="launch-result" class="muted" style="margin-top:10px"></div>
    </section>
    <section class="card"><h2>Bindings</h2><pre id="bindings"></pre></section>
    <section class="card"><h2>Queues</h2><pre id="queues"></pre></section>
    <section class="card"><h2>Reactions</h2><pre id="reactions"></pre></section>
    <section class="card"><h2>Launched Agents</h2><div id="launched-controls"></div><pre id="launched"></pre></section>
    <section class="card"><h2>Harness Log Preview</h2><div id="ptylogs"></div></section>
    <section class="card"><h2>PTY Input Preview</h2><div id="ptyinputs"></div></section>
    <section class="card"><h2>PTY Transcript Preview</h2><div id="ptyoutputs"></div></section>
    <section class="card"><h2>Recent Chats</h2><div id="chats"></div></section>
    <section class="card"><h2>Recent Attachments</h2><div id="attachments"></div></section>
    <section class="card"><h2>Audit Tail</h2><div id="audit"></div></section>
  </main>
  <script>
    const pretty = (v) => JSON.stringify(v, null, 2);
    const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
    function renderRows(items, mapFn) {
      if (!items || !items.length) return '<div class="muted">None yet.</div>';
      return '<table><tbody>' + items.map(mapFn).join('') + '</tbody></table>';
    }
    async function refresh() {
      const res = await fetch('/api/dashboard');
      const data = await res.json();
      document.getElementById('bridge').textContent = pretty({ envelope: data.envelope, dryRun: data.dryRun, now: data.now, defaultGuildId: data.defaultGuildId, assignableChannelIds: data.assignableChannelIds, statusReactions: data.statusReactions, finalChoices: data.finalReactionChoices });
      const guildInput = document.querySelector('input[name="guildId"]');
      if (guildInput && !guildInput.value && data.defaultGuildId) guildInput.value = data.defaultGuildId;
      const select = document.getElementById('channel-select');
      if (select) {
        const current = select.value;
        select.innerHTML = '<option value="">Auto-assign</option>' + (data.assignableChannels || []).map((ch) => '<option value="' + esc(ch.id) + '">' + esc(ch.label || ch.id) + '</option>').join('');
        if (current) select.value = current;
      }
      document.getElementById('bindings').textContent = pretty(data.bindings);
      document.getElementById('queues').textContent = pretty(data.queueSizes);
      document.getElementById('reactions').textContent = pretty(data.managedReactions);
      document.getElementById('launched').textContent = pretty(data.launchedAgents);
      const launchedEntries = Object.entries(data.launchedAgents || {});
      document.getElementById('launched-controls').innerHTML = launchedEntries.length ? launchedEntries.map(([agentId, agent]) => '<div style="margin:0 0 8px 0"><button data-stop-agent="' + esc(agentId) + '"' + ((agent && agent.state === 'running') ? '' : ' disabled') + '>Stop ' + esc(agentId) + '</button> <span class="muted">' + esc((agent && agent.state) || 'unknown') + '</span></div>').join('') : '<div class="muted">No launched agents.</div>';
      const logEntries = Object.entries(data.launchedAgentLogs || {});
      const inputEntries = Object.entries(data.launchedAgentPtyInputs || {});
      const outputEntries = Object.entries(data.launchedAgentPtyOutputs || {});
      document.getElementById('ptylogs').innerHTML = logEntries.length ? logEntries.map(([agentId, lines]) => '<h3>' + esc(agentId) + '</h3><pre>' + esc((lines || []).join('\n')) + '</pre>').join('') : '<div class="muted">No launched agent logs yet.</div>';
      document.getElementById('ptyinputs').innerHTML = inputEntries.length ? inputEntries.map(([agentId, lines]) => '<h3>' + esc(agentId) + '</h3><pre>' + esc((lines || []).join('\n')) + '</pre>').join('') : '<div class="muted">No PTY input transcript yet.</div>';
      document.getElementById('ptyoutputs').innerHTML = outputEntries.length ? outputEntries.map(([agentId, lines]) => '<h3>' + esc(agentId) + '</h3><pre>' + esc((lines || []).join('\n')) + '</pre>').join('') : '<div class="muted">No PTY transcript yet.</div>';
      document.getElementById('chats').innerHTML = renderRows(data.recentChats, (item) => '<tr><td><strong>' + esc(item.authorName || item.authorId || 'unknown') + '</strong><div class="muted">' + esc(item.channelId) + ' · ' + esc(item.timestamp) + '</div></td><td>' + esc(item.content || '') + '</td></tr>');
      document.getElementById('attachments').innerHTML = '<div class="muted">Total files: ' + esc(data.attachmentCount) + '</div>' + renderRows(data.recentAttachments, (item) => '<tr><td><strong>' + esc(item.filename) + '</strong></td><td>' + esc(item.localPath || '') + '</td></tr>');
      document.getElementById('audit').innerHTML = renderRows(data.auditTail, (item) => '<tr><td><strong>' + esc(item.type) + '</strong><div class="muted">' + esc(item.timestamp) + '</div></td><td><pre>' + esc(JSON.stringify(item.payload, null, 2)) + '</pre></td></tr>');
    }
    document.addEventListener('click', async (e) => {
      const btn = e.target.closest('[data-stop-agent]');
      if (!btn) return;
      const agentId = btn.getAttribute('data-stop-agent');
      const res = await fetch('/api/stop-agent', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agentId }) });
      const text = await res.text();
      document.getElementById('launch-result').textContent = res.ok ? 'Stopped: ' + text : 'Stop failed: ' + text;
      refresh();
    });
    document.getElementById('launch-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const form = new FormData(e.target);
      const payload = {
        agentId: String(form.get('agentId') || '').trim(),
        guildId: String(form.get('guildId') || '').trim(),
        channelId: String(form.get('channelId') || '').trim(),
        command: String(form.get('command') || '').trim()
      };
      const res = await fetch('/api/launch-agent', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      const text = await res.text();
      document.getElementById('launch-result').textContent = res.ok ? 'Launched: ' + text : 'Launch failed: ' + text;
      refresh();
    });
    refresh();
    setInterval(refresh, 5000);
  </script>
</body>
</html>`
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
