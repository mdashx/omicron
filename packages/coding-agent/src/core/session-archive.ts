import { randomBytes } from "node:crypto";
import { closeSync, fsyncSync, mkdirSync, openSync, writeSync } from "node:fs";
import { hostname } from "node:os";
import { dirname, isAbsolute, join } from "node:path";
import { getAgentDir, VERSION } from "../config.ts";
import { resolvePath } from "../utils/paths.ts";
import type { AgentSessionEvent } from "./agent-session.ts";

export type SessionArchiveEventType =
	| "session_start"
	| "message"
	| "tool_call"
	| "tool_result"
	| "session_end"
	| "error";
export type SessionArchiveRole = "user" | "assistant" | "tool" | "system";
export type SessionArchiveRedactMode = "minimal" | "strict";

export interface SessionArchiveConfig {
	enabled?: boolean;
	repoPath?: string;
	fileLayout?: string;
	outputFormat?: string;
	captureEvents?: SessionArchiveEventType[];
	redactMode?: SessionArchiveRedactMode;
	failClosed?: boolean;
}

export interface ResolvedSessionArchiveConfig {
	enabled: boolean;
	repoPath: string;
	fileLayout: string;
	outputFormat: "jsonl";
	captureEvents: SessionArchiveEventType[];
	redactMode: SessionArchiveRedactMode;
	failClosed: boolean;
}

export interface PiSessionEnvelope {
	sessionId: string;
	startedAt: string;
	source: string;
	host: string;
	runtimeVersion: string;
	packageVersion: string;
	mode: string;
	cwd: string;
}

export interface PiArchiveEvent {
	sessionId: string;
	eventId: string;
	timestamp: string;
	role: SessionArchiveRole;
	eventType: SessionArchiveEventType;
	content: string;
	metadata: string;
}

export interface SessionArchiveRecordInput {
	eventType: SessionArchiveEventType;
	role: SessionArchiveRole;
	content: string;
	metadata?: Record<string, unknown>;
}

export interface SessionArchiveRuntimeOptions {
	config?: SessionArchiveConfig;
	repoPath?: string;
	cwd: string;
	agentDir?: string;
	mode: string;
	sessionStartReason?: string;
	packageVersion?: string;
	runtimeVersion?: string;
	source?: string;
	envelope?: Partial<PiSessionEnvelope>;
}

const DEFAULT_CAPTURE_EVENTS: SessionArchiveEventType[] = [
	"session_start",
	"message",
	"tool_call",
	"tool_result",
	"session_end",
	"error",
];

function pad2(value: number): string {
	return String(value).padStart(2, "0");
}

function createSessionId(): string {
	const now = new Date();
	const iso = now
		.toISOString()
		.replace(/\.\d{3}Z$/, "Z")
		.replace(/:/g, "-");
	return `${iso}_${randomBytes(2).toString("hex")}`;
}

function toUtcDateParts(timestamp: string): { yyyy: string; mm: string; dd: string } {
	const date = new Date(timestamp);
	if (Number.isNaN(date.getTime())) {
		return { yyyy: "1970", mm: "01", dd: "01" };
	}
	return {
		yyyy: String(date.getUTCFullYear()),
		mm: pad2(date.getUTCMonth() + 1),
		dd: pad2(date.getUTCDate()),
	};
}

function sanitizeSessionId(sessionId: string): string {
	return sessionId.replace(/[:/\\]/g, "-");
}

function resolveRepoPath(repoPath: string, agentDir: string): string {
	const expanded = repoPath.trim();
	if (!expanded) {
		throw new Error("Session archive repoPath must not be empty");
	}
	return resolvePath(expanded, agentDir);
}

function resolveArchiveFilePath(config: ResolvedSessionArchiveConfig, envelope: PiSessionEnvelope): string {
	const parts = toUtcDateParts(envelope.startedAt);
	const fileSafeSessionId = sanitizeSessionId(envelope.sessionId);
	const layout = config.fileLayout
		.replace(/yyyy/g, parts.yyyy)
		.replace(/mm/g, parts.mm)
		.replace(/dd/g, parts.dd)
		.replace(/sessionId/g, fileSafeSessionId);
	const normalizedLayout = layout.replace(/^[/\\]+/, "");
	return join(config.repoPath, normalizedLayout);
}

function isSupportedEventType(value: string): value is SessionArchiveEventType {
	return DEFAULT_CAPTURE_EVENTS.includes(value as SessionArchiveEventType);
}

function normalizeCaptureEvents(events: SessionArchiveEventType[] | undefined): SessionArchiveEventType[] {
	if (events === undefined) {
		return [...DEFAULT_CAPTURE_EVENTS];
	}
	const unique = new Set<SessionArchiveEventType>();
	for (const eventType of events) {
		if (!isSupportedEventType(eventType)) {
			throw new Error(`Invalid session archive capture event: ${String(eventType)}`);
		}
		unique.add(eventType);
	}
	return [...unique];
}

function validateResolvedConfig(config: ResolvedSessionArchiveConfig): void {
	if (!config.enabled) {
		return;
	}
	if (config.outputFormat !== "jsonl") {
		throw new Error(`Unsupported session archive output format: ${config.outputFormat}`);
	}
	if (!config.repoPath.trim()) {
		throw new Error("Session archive repoPath must not be empty");
	}
	if (!config.fileLayout.trim()) {
		throw new Error("Session archive fileLayout must not be empty");
	}
	if (isAbsolute(config.fileLayout)) {
		throw new Error("Session archive fileLayout must be relative to repoPath");
	}
	if (/(^|[\\/])\.\.([\\/]|$)/.test(config.fileLayout)) {
		throw new Error("Session archive fileLayout must not escape repoPath");
	}
	if (!config.captureEvents.every(isSupportedEventType)) {
		throw new Error("Session archive captureEvents contains unsupported values");
	}
	if (config.redactMode !== "minimal" && config.redactMode !== "strict") {
		throw new Error(`Unsupported session archive redactMode: ${config.redactMode}`);
	}
}

function parseTruthy(value: string | undefined): boolean | undefined {
	if (value === undefined) return undefined;
	if (["1", "true", "yes", "on"].includes(value.toLowerCase())) return true;
	if (["0", "false", "no", "off"].includes(value.toLowerCase())) return false;
	throw new Error(`Invalid boolean value: ${value}`);
}

function parseCaptureEvents(value: string | undefined): SessionArchiveEventType[] | undefined {
	if (value === undefined) return undefined;
	const items = value
		.split(",")
		.map((part) => part.trim())
		.filter((part) => part.length > 0);
	return items.length > 0 ? (items as SessionArchiveEventType[]) : [];
}

function resolveArchiveEnvOverrides(): SessionArchiveConfig {
	const overrides: SessionArchiveConfig = {};
	const enabled = parseTruthy(process.env.PI_SESSION_ARCHIVE_ENABLED);
	if (enabled !== undefined) overrides.enabled = enabled;
	if (process.env.PI_SESSION_ARCHIVE_REPO_PATH) overrides.repoPath = process.env.PI_SESSION_ARCHIVE_REPO_PATH;
	if (process.env.PI_SESSION_ARCHIVE_FILE_LAYOUT) overrides.fileLayout = process.env.PI_SESSION_ARCHIVE_FILE_LAYOUT;
	if (process.env.PI_SESSION_ARCHIVE_OUTPUT_FORMAT)
		overrides.outputFormat = process.env.PI_SESSION_ARCHIVE_OUTPUT_FORMAT;
	const captureEvents = parseCaptureEvents(process.env.PI_SESSION_ARCHIVE_CAPTURE_EVENTS);
	if (captureEvents !== undefined) overrides.captureEvents = captureEvents;
	if (process.env.PI_SESSION_ARCHIVE_REDACT_MODE) {
		overrides.redactMode = process.env.PI_SESSION_ARCHIVE_REDACT_MODE as SessionArchiveRedactMode;
	}
	const failClosed = parseTruthy(process.env.PI_SESSION_ARCHIVE_FAIL_CLOSED);
	if (failClosed !== undefined) overrides.failClosed = failClosed;
	return overrides;
}

export const SessionArchiveDefaults = {
	enabled: true,
	repoPath: "~/.pi/agent/session-archive",
	fileLayout: "yyyy/mm/dd/sessionId.jsonl",
	outputFormat: "jsonl",
	captureEvents: [...DEFAULT_CAPTURE_EVENTS],
	redactMode: "minimal",
	failClosed: true,
} as const;

export const PiArchiveEnforcement = {
	pluginLoaded: true,
	hooksInstalled: true,
	schemaValidated: true,
	startupBlockedIfMissing: true,
	llmInPath: false,
} as const;

export function resolveSessionArchiveConfig(
	options: { config?: SessionArchiveConfig; agentDir?: string; cwd?: string } = {},
): ResolvedSessionArchiveConfig {
	const agentDir = resolvePath(options.agentDir ?? getAgentDir());
	const defaults: ResolvedSessionArchiveConfig = {
		enabled: SessionArchiveDefaults.enabled,
		repoPath: resolveRepoPath(SessionArchiveDefaults.repoPath, agentDir),
		fileLayout: SessionArchiveDefaults.fileLayout,
		outputFormat: SessionArchiveDefaults.outputFormat,
		captureEvents: [...SessionArchiveDefaults.captureEvents],
		redactMode: SessionArchiveDefaults.redactMode,
		failClosed: SessionArchiveDefaults.failClosed,
	};
	const merged: SessionArchiveConfig = {
		...defaults,
		...(options.config ?? {}),
		...resolveArchiveEnvOverrides(),
	};
	const resolved: ResolvedSessionArchiveConfig = {
		enabled: merged.enabled ?? defaults.enabled,
		repoPath: resolveRepoPath(merged.repoPath ?? defaults.repoPath, agentDir),
		fileLayout: merged.fileLayout ?? defaults.fileLayout,
		outputFormat: (merged.outputFormat ?? defaults.outputFormat) as "jsonl",
		captureEvents: normalizeCaptureEvents(merged.captureEvents ?? defaults.captureEvents),
		redactMode: (merged.redactMode ?? defaults.redactMode) as SessionArchiveRedactMode,
		failClosed: merged.failClosed ?? defaults.failClosed,
	};
	validateResolvedConfig(resolved);
	return resolved;
}

function safeJsonStringify(value: unknown): string {
	try {
		return JSON.stringify(value);
	} catch {
		return JSON.stringify({ error: "unserializable" });
	}
}

function extractMessageText(content: unknown): string {
	if (typeof content === "string") return content;
	if (Array.isArray(content)) {
		return content
			.map((part) => {
				if (part && typeof part === "object" && "type" in part) {
					const record = part as Record<string, unknown>;
					if (record.type === "text" && typeof record.text === "string") return record.text;
				}
				return safeJsonStringify(part);
			})
			.join("");
	}
	return safeJsonStringify(content);
}

function buildEnvelope(options: SessionArchiveRuntimeOptions, sessionId: string): PiSessionEnvelope {
	const startedAt = new Date().toISOString();
	return Object.freeze({
		sessionId,
		startedAt,
		source: options.source ?? "pi-coding-agent",
		host: hostname(),
		runtimeVersion: options.runtimeVersion ?? VERSION,
		packageVersion: options.packageVersion ?? VERSION,
		mode: options.mode,
		cwd: resolvePath(options.cwd),
		...options.envelope,
	}) as PiSessionEnvelope;
}

function buildRecordId(counter: number): string {
	return `evt_${String(counter).padStart(4, "0")}`;
}

export class PiArchiveWriter {
	private fd: number;
	readonly config: ResolvedSessionArchiveConfig;
	readonly envelope: PiSessionEnvelope;
	readonly filePath: string;
	readonly appendOnly = true;
	flushed = true;

	constructor(config: ResolvedSessionArchiveConfig, envelope: PiSessionEnvelope, startReason: string) {
		this.config = config;
		this.envelope = envelope;
		this.filePath = resolveArchiveFilePath(config, envelope);
		const dir = dirname(this.filePath);
		mkdirSync(dir, { recursive: true });
		this.fd = openSync(this.filePath, "a");
		this.appendRecord({
			eventType: "session_start",
			role: "system",
			content: safeJsonStringify(envelope),
			metadata: { reason: startReason },
		});
	}

	get path(): string {
		return this.filePath;
	}

	appendRecord(input: SessionArchiveRecordInput): void {
		const event: PiArchiveEvent = {
			sessionId: this.envelope.sessionId,
			eventId: buildRecordId(this.nextEventId()),
			timestamp: new Date().toISOString(),
			role: input.role,
			eventType: input.eventType,
			content: input.content,
			metadata: safeJsonStringify(input.metadata ?? {}),
		};
		this.validateRecord(event);
		const line = `${JSON.stringify(event)}\n`;
		writeSync(this.fd, line);
		fsyncSync(this.fd);
		this.flushed = true;
	}

	appendSessionEnd(reason: string, targetSessionFile?: string): void {
		this.appendRecord({
			eventType: "session_end",
			role: "system",
			content: reason,
			metadata: targetSessionFile ? { reason, targetSessionFile } : { reason },
		});
	}

	close(): void {
		try {
			fsyncSync(this.fd);
		} catch {
			// ignore flush errors on close; the write path already failed closed
		}
		closeSync(this.fd);
	}

	private eventCounter = 0;
	private nextEventId(): number {
		this.eventCounter += 1;
		return this.eventCounter;
	}

	private validateRecord(event: PiArchiveEvent): void {
		if (!event.sessionId || !event.eventId || !event.timestamp) {
			throw new Error("Invalid archive record");
		}
		if (!event.content && event.eventType !== "session_start") {
			throw new Error(`Archive record ${event.eventType} must have content`);
		}
	}
}

export class SessionArchiveRuntime {
	private readonly writer?: PiArchiveWriter;
	private readonly unsubscribe?: () => void;
	private disposed = false;
	readonly config: ResolvedSessionArchiveConfig;
	readonly envelope: PiSessionEnvelope;
	readonly filePath?: string;
	private readonly session: { subscribe(listener: (event: AgentSessionEvent) => void): () => void };

	constructor(
		session: { subscribe(listener: (event: AgentSessionEvent) => void): () => void },
		options: SessionArchiveRuntimeOptions,
	) {
		this.session = session;
		this.config = resolveSessionArchiveConfig({
			config: options.config,
			agentDir: options.agentDir ?? getAgentDir(),
		});
		this.envelope = buildEnvelope(options, createSessionId());
		if (!this.config.enabled) {
			return;
		}
		this.writer = new PiArchiveWriter(this.config, this.envelope, options.sessionStartReason ?? "startup");
		this.filePath = this.writer.path;
		this.unsubscribe = this.session.subscribe((event) => this.handleEvent(event));
	}

	handleEvent(event: AgentSessionEvent): void {
		if (!this.writer || this.disposed) return;
		try {
			switch (event.type) {
				case "message_start":
					if (event.message.role === "user" && this.config.captureEvents.includes("message")) {
						this.writer.appendRecord({
							eventType: "message",
							role: "user",
							content: extractMessageText(event.message.content),
							metadata: { phase: "start" },
						});
					}
					break;
				case "message_end":
					if (event.message.role === "assistant") {
						const assistant = event.message as unknown as Record<string, unknown>;
						if ((assistant.stopReason as string | undefined) === "error") {
							if (this.config.captureEvents.includes("error")) {
								this.writer.appendRecord({
									eventType: "error",
									role: "assistant",
									content: String(assistant.errorMessage ?? "assistant error"),
									metadata: { stopReason: "error" },
								});
							}
						} else if (this.config.captureEvents.includes("message")) {
							this.writer.appendRecord({
								eventType: "message",
								role: "assistant",
								content: extractMessageText(event.message.content),
								metadata: {
									provider: (assistant.provider as string | undefined) ?? undefined,
									model: (assistant.model as string | undefined) ?? undefined,
								},
							});
						}
					}
					break;
				case "tool_execution_start":
					if (this.config.captureEvents.includes("tool_call")) {
						this.writer.appendRecord({
							eventType: "tool_call",
							role: "tool",
							content: safeJsonStringify(event.args),
							metadata: { toolName: event.toolName, toolCallId: event.toolCallId },
						});
					}
					break;
				case "tool_execution_end":
					if (event.isError) {
						if (this.config.captureEvents.includes("error")) {
							this.writer.appendRecord({
								eventType: "error",
								role: "tool",
								content: safeJsonStringify(event.result),
								metadata: { toolName: event.toolName, toolCallId: event.toolCallId, isError: true },
							});
						}
					} else if (this.config.captureEvents.includes("tool_result")) {
						this.writer.appendRecord({
							eventType: "tool_result",
							role: "tool",
							content: safeJsonStringify(event.result),
							metadata: { toolName: event.toolName, toolCallId: event.toolCallId },
						});
					}
					break;
			}
		} catch (error) {
			if (this.config.failClosed) {
				throw error;
			}
		}
	}

	endSession(reason: string, targetSessionFile?: string): void {
		if (!this.writer || this.disposed) return;
		this.writer.appendSessionEnd(reason, targetSessionFile);
	}

	dispose(reason: string, targetSessionFile?: string): void {
		if (this.disposed) return;
		this.disposed = true;
		try {
			this.endSession(reason, targetSessionFile);
		} finally {
			this.unsubscribe?.();
			this.writer?.close();
		}
	}
}

export function createSessionArchiveRuntime(
	session: { subscribe(listener: (event: AgentSessionEvent) => void): () => void },
	options: SessionArchiveRuntimeOptions,
): SessionArchiveRuntime | undefined {
	const config = resolveSessionArchiveConfig({ config: options.config, agentDir: options.agentDir ?? getAgentDir() });
	if (!config.enabled) return undefined;
	return new SessionArchiveRuntime(session, options);
}
