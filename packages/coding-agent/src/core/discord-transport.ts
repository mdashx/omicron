import { createHash, randomBytes } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { Client, GatewayIntentBits } from "discord.js";
import { CONFIG_DIR_NAME, expandTildePath, VERSION } from "../config.ts";
import type { AgentSession } from "./agent-session.ts";

export interface DiscordTransportDefaults {
	enabled: boolean;
	tokenSource: string;
	commandPrefix: string;
	guildAllowlist: string[];
	channelAllowlist: string[];
	dryRun: boolean;
	statePath: string;
	failClosed: boolean;
}

export interface DiscordTransportConfig {
	enabled?: boolean;
	tokenSource?: string;
	commandPrefix?: string;
	guildAllowlist?: string[];
	channelAllowlist?: string[];
	ownerAllowlist?: string[];
	dryRun?: boolean;
	statePath?: string;
	threadMode?: string;
	replyMode?: string;
	maxActionsPerTurn?: number;
	failClosed?: boolean;
}

export interface ResolvedDiscordTransportConfig {
	enabled: boolean;
	tokenSource: string;
	commandPrefix: string;
	guildAllowlist: string[];
	channelAllowlist: string[];
	ownerAllowlist: string[];
	dryRun: boolean;
	statePath: string;
	threadMode: string;
	replyMode: string;
	maxActionsPerTurn: number;
	failClosed: boolean;
}

export interface DiscordSessionEnvelope {
	sessionId: string;
	startedAt: string;
	source: string;
	host: string;
	runtimeVersion: string;
	packageVersion: string;
	botUserId: string;
	applicationId: string;
	transportMode: string;
}

export type DiscordInboundEventType = "ready" | "messageCreate" | "reactionAdd" | "error" | "heartbeat";

export interface DiscordInboundEvent {
	eventId: string;
	eventType: DiscordInboundEventType;
	guildId: string;
	channelId: string;
	messageId: string;
	authorId: string;
	content: string;
	createdAt: string;
	jumpUrl: string;
	rawKind: string;
}

export type DiscordDirectiveType = "ping" | "status" | "echo" | "react" | "ignore";

export interface DiscordDirective {
	directiveType: DiscordDirectiveType;
	sourceEventId: string;
	targetChannelId: string;
	argumentText: string;
}

export type DiscordActionType = "reply" | "addReaction" | "openThread" | "logOnly" | "noOp";

export interface DiscordActionPlan {
	actionId: string;
	sourceEventId: string;
	actionType: DiscordActionType;
	targetChannelId: string;
	targetMessageId: string;
	bodyText: string;
	emoji: string;
}

export interface DiscordTransportState {
	lastReadyAt: string;
	lastHeartbeatAt: string;
	processedEventIds: Set<string>;
	processedActionIds: Set<string>;
	lastSeenByChannel: string;
}

export interface DiscordTransportMessageLike {
	id: string;
	channelId: string;
	guildId: string | null;
	author: { id: string; bot?: boolean | null };
	content: string;
	createdTimestamp: number;
	url: string;
	reply(content: string): Promise<unknown>;
	react(emoji: string): Promise<unknown>;
}

export interface DiscordTransportReactionLike {
	message: { id: string; channelId: string; guildId: string | null };
	emoji: { name?: string | null; id?: string | null; toString(): string };
}

export interface DiscordClientIdentity {
	botUserId: string;
	applicationId: string;
}

export interface DiscordTransportClientAdapter {
	start(token: string): Promise<void>;
	stop(): Promise<void>;
	onReady(handler: (identity: DiscordClientIdentity) => void): () => void;
	onMessageCreate(handler: (message: DiscordTransportMessageLike) => void): () => void;
	onReactionAdd(handler: (reaction: DiscordTransportReactionLike, userId: string) => void): () => void;
	onError(handler: (error: Error) => void): () => void;
}

export interface DiscordTransportSessionLike extends Pick<AgentSession, "sendCustomMessage"> {}

export interface DiscordTransportOptions {
	config: ResolvedDiscordTransportConfig;
	session: DiscordTransportSessionLike;
	client?: DiscordTransportClientAdapter;
	host?: string;
	runtimeVersion?: string;
	packageVersion?: string;
}

function uniqueStrings(values: readonly string[]): string[] {
	return [...new Set(values.map((value) => value.trim()).filter((value) => value.length > 0))];
}

function parseList(value: string | undefined): string[] | undefined {
	if (value === undefined) {
		return undefined;
	}
	if (value.trim() === "") {
		return [];
	}
	return uniqueStrings(value.split(","));
}

function parseBoolean(value: string | undefined): boolean | undefined {
	if (value === undefined) {
		return undefined;
	}
	const normalized = value.trim().toLowerCase();
	if (normalized === "1" || normalized === "true" || normalized === "yes" || normalized === "on") {
		return true;
	}
	if (normalized === "0" || normalized === "false" || normalized === "no" || normalized === "off") {
		return false;
	}
	return undefined;
}

function parseNumber(value: string | undefined): number | undefined {
	if (value === undefined) {
		return undefined;
	}
	const parsed = Number(value);
	if (!Number.isFinite(parsed) || !Number.isInteger(parsed)) {
		return undefined;
	}
	return parsed;
}

export function getDiscordTransportDefaults(): DiscordTransportDefaults {
	return {
		enabled: true,
		tokenSource: "env.DISCORD_BOT_TOKEN",
		commandPrefix: "!",
		guildAllowlist: [],
		channelAllowlist: [],
		dryRun: true,
		statePath: join(homedir(), CONFIG_DIR_NAME, "discord-transport", "state.json"),
		failClosed: true,
	};
}

export function resolveDiscordTransportConfig(
	overrides: DiscordTransportConfig | undefined,
	env: NodeJS.ProcessEnv = process.env,
): ResolvedDiscordTransportConfig {
	const defaults = getDiscordTransportDefaults();
	const envTokenSource = env.PI_DISCORD_TOKEN_SOURCE;
	const envStatePath = env.PI_DISCORD_STATE_PATH;
	const envThreadMode = env.PI_DISCORD_THREAD_MODE;
	const envReplyMode = env.PI_DISCORD_REPLY_MODE;
	const envConfig: DiscordTransportConfig = {
		enabled: parseBoolean(env.PI_DISCORD_ENABLED),
		tokenSource: envTokenSource,
		commandPrefix: env.PI_DISCORD_COMMAND_PREFIX,
		guildAllowlist: parseList(env.PI_DISCORD_GUILD_ALLOWLIST),
		channelAllowlist: parseList(env.PI_DISCORD_CHANNEL_ALLOWLIST),
		ownerAllowlist: parseList(env.PI_DISCORD_OWNER_ALLOWLIST),
		dryRun: parseBoolean(env.PI_DISCORD_DRY_RUN),
		statePath: envStatePath,
		threadMode: envThreadMode,
		replyMode: envReplyMode,
		maxActionsPerTurn: parseNumber(env.PI_DISCORD_MAX_ACTIONS_PER_TURN),
		failClosed: parseBoolean(env.PI_DISCORD_FAIL_CLOSED),
	};
	const merged: DiscordTransportConfig = {
		...defaults,
		...overrides,
	};
	if (envConfig.enabled !== undefined) merged.enabled = envConfig.enabled;
	if (envConfig.tokenSource !== undefined) merged.tokenSource = envConfig.tokenSource;
	if (envConfig.commandPrefix !== undefined) merged.commandPrefix = envConfig.commandPrefix;
	if (envConfig.guildAllowlist !== undefined) merged.guildAllowlist = envConfig.guildAllowlist;
	if (envConfig.channelAllowlist !== undefined) merged.channelAllowlist = envConfig.channelAllowlist;
	if (envConfig.ownerAllowlist !== undefined) merged.ownerAllowlist = envConfig.ownerAllowlist;
	if (envConfig.dryRun !== undefined) merged.dryRun = envConfig.dryRun;
	if (envConfig.statePath !== undefined) merged.statePath = envConfig.statePath;
	if (envConfig.threadMode !== undefined) merged.threadMode = envConfig.threadMode;
	if (envConfig.replyMode !== undefined) merged.replyMode = envConfig.replyMode;
	if (envConfig.maxActionsPerTurn !== undefined) merged.maxActionsPerTurn = envConfig.maxActionsPerTurn;
	if (envConfig.failClosed !== undefined) merged.failClosed = envConfig.failClosed;
	const statePath = merged.statePath ? expandTildePath(merged.statePath) : defaults.statePath;
	const tokenSource = merged.tokenSource?.trim() || defaults.tokenSource;
	const commandPrefix = merged.commandPrefix?.trim() || defaults.commandPrefix;
	const guildAllowlist = uniqueStrings(merged.guildAllowlist ?? defaults.guildAllowlist);
	const channelAllowlist = uniqueStrings(merged.channelAllowlist ?? defaults.channelAllowlist);
	const ownerAllowlist = uniqueStrings(merged.ownerAllowlist ?? []);
	const threadMode = merged.threadMode?.trim() || "off";
	const replyMode = merged.replyMode?.trim() || "reply-to-message";
	const maxActionsPerTurn = merged.maxActionsPerTurn ?? 8;
	const enabled = merged.enabled ?? defaults.enabled;
	const dryRun = merged.dryRun ?? defaults.dryRun;
	const failClosed = merged.failClosed ?? defaults.failClosed;

	const errors: string[] = [];
	if (enabled && tokenSource.length === 0) errors.push("tokenSource must not be empty");
	if (enabled && commandPrefix.length === 0) errors.push("commandPrefix must not be empty");
	if (enabled && statePath.length === 0) errors.push("statePath must not be empty");
	if (enabled && maxActionsPerTurn <= 0) errors.push("maxActionsPerTurn must be greater than 0");
	if (errors.length > 0) {
		throw new Error(`Invalid Discord transport config: ${errors.join(", ")}`);
	}

	return {
		enabled,
		tokenSource,
		commandPrefix,
		guildAllowlist,
		channelAllowlist,
		ownerAllowlist,
		dryRun,
		statePath,
		threadMode,
		replyMode,
		maxActionsPerTurn,
		failClosed,
	};
}

export async function resolveDiscordToken(
	tokenSource: string,
	env: NodeJS.ProcessEnv = process.env,
): Promise<string | undefined> {
	const source = tokenSource.trim();
	if (source.startsWith("file:")) {
		const filePath = source.slice("file:".length).trim();
		if (filePath.length === 0) {
			return undefined;
		}
		const raw = await readFile(filePath, "utf8");
		return raw.trim() || undefined;
	}
	if (source.startsWith("env.")) {
		return env[source.slice(4)]?.trim() || undefined;
	}
	if (source.startsWith("$")) {
		return env[source.slice(1)]?.trim() || undefined;
	}
	return env[source]?.trim() || source || undefined;
}

export function createDiscordSessionEnvelope(options: {
	host?: string;
	runtimeVersion?: string;
	packageVersion?: string;
	botUserId: string;
	applicationId: string;
	transportMode: string;
}): DiscordSessionEnvelope {
	const startedAt = new Date().toISOString();
	const suffix = randomBytes(2).toString("hex");
	return {
		sessionId: `${startedAt.replace(/[:.]/g, "-")}_${suffix}`,
		startedAt,
		source: "pi-discord-transport",
		host: options.host ?? process.env.HOSTNAME ?? process.env.COMPUTERNAME ?? "unknown-host",
		runtimeVersion: options.runtimeVersion ?? `${process.release.name}/${process.version}`,
		packageVersion: options.packageVersion ?? VERSION,
		botUserId: options.botUserId,
		applicationId: options.applicationId,
		transportMode: options.transportMode,
	};
}

export async function loadDiscordTransportState(statePath: string): Promise<DiscordTransportState> {
	try {
		const raw = await readFile(statePath, "utf8");
		const parsed = JSON.parse(raw) as {
			lastReadyAt?: string;
			lastHeartbeatAt?: string;
			processedEventIds?: string[];
			processedActionIds?: string[];
			lastSeenByChannel?: string;
		};
		return {
			lastReadyAt: parsed.lastReadyAt ?? "",
			lastHeartbeatAt: parsed.lastHeartbeatAt ?? "",
			processedEventIds: new Set(parsed.processedEventIds ?? []),
			processedActionIds: new Set(parsed.processedActionIds ?? []),
			lastSeenByChannel: parsed.lastSeenByChannel ?? "{}",
		};
	} catch (error) {
		if (error instanceof Error && "code" in error && (error as NodeJS.ErrnoException).code === "ENOENT") {
			return {
				lastReadyAt: "",
				lastHeartbeatAt: "",
				processedEventIds: new Set(),
				processedActionIds: new Set(),
				lastSeenByChannel: "{}",
			};
		}
		throw error;
	}
}

export async function saveDiscordTransportState(statePath: string, state: DiscordTransportState): Promise<void> {
	await mkdir(dirname(statePath), { recursive: true });
	const payload = {
		lastReadyAt: state.lastReadyAt,
		lastHeartbeatAt: state.lastHeartbeatAt,
		processedEventIds: [...state.processedEventIds],
		processedActionIds: [...state.processedActionIds],
		lastSeenByChannel: state.lastSeenByChannel,
	};
	await writeFile(statePath, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
}

export function normalizeDiscordMessage(message: DiscordTransportMessageLike): DiscordInboundEvent {
	return {
		eventId: `msg_${message.id}`,
		eventType: "messageCreate",
		guildId: message.guildId ?? "",
		channelId: message.channelId,
		messageId: message.id,
		authorId: message.author.id,
		content: message.content,
		createdAt: new Date(message.createdTimestamp).toISOString(),
		jumpUrl: message.url,
		rawKind: "gateway.messageCreate",
	};
}

export function normalizeDiscordReaction(reaction: DiscordTransportReactionLike, userId: string): DiscordInboundEvent {
	return {
		eventId: `react_${reaction.message.id}_${createHash("sha1").update(`${userId}:${reaction.emoji.toString()}`).digest("hex").slice(0, 8)}`,
		eventType: "reactionAdd",
		guildId: reaction.message.guildId ?? "",
		channelId: reaction.message.channelId,
		messageId: reaction.message.id,
		authorId: userId,
		content: reaction.emoji.toString(),
		createdAt: new Date().toISOString(),
		jumpUrl: "",
		rawKind: "gateway.reactionAdd",
	};
}

export function createDiscordHeartbeatEvent(envelope: DiscordSessionEnvelope): DiscordInboundEvent {
	const createdAt = new Date().toISOString();
	return {
		eventId: `heartbeat_${envelope.sessionId}_${createdAt}`,
		eventType: "heartbeat",
		guildId: "",
		channelId: "",
		messageId: "",
		authorId: "",
		content: envelope.transportMode,
		createdAt,
		jumpUrl: "",
		rawKind: "transport.heartbeat",
	};
}

export function normalizeDiscordReadyEvent(envelope: DiscordSessionEnvelope): DiscordInboundEvent {
	return {
		eventId: `ready_${envelope.sessionId}`,
		eventType: "ready",
		guildId: "",
		channelId: "",
		messageId: "",
		authorId: envelope.botUserId,
		content: envelope.applicationId,
		createdAt: envelope.startedAt,
		jumpUrl: "",
		rawKind: "gateway.ready",
	};
}

export function parseDiscordDirective(
	prefix: string,
	content: string,
	sourceEventId: string,
	channelId: string,
): DiscordDirective {
	if (!content.startsWith(prefix)) {
		return { directiveType: "ignore", sourceEventId, targetChannelId: channelId, argumentText: "" };
	}
	const rest = content.slice(prefix.length).trim();
	if (rest.length === 0) {
		return { directiveType: "ignore", sourceEventId, targetChannelId: channelId, argumentText: "" };
	}
	const [command, ...args] = rest.split(/\s+/);
	const argumentText = args.join(" ").trim();
	if (command === "ping")
		return { directiveType: "ping", sourceEventId, targetChannelId: channelId, argumentText: "" };
	if (command === "status")
		return { directiveType: "status", sourceEventId, targetChannelId: channelId, argumentText: "" };
	if (command === "echo") return { directiveType: "echo", sourceEventId, targetChannelId: channelId, argumentText };
	if (command === "react") return { directiveType: "react", sourceEventId, targetChannelId: channelId, argumentText };
	return { directiveType: "ignore", sourceEventId, targetChannelId: channelId, argumentText: "" };
}

export function planDiscordActions(
	config: ResolvedDiscordTransportConfig,
	envelope: DiscordSessionEnvelope,
	event: DiscordInboundEvent,
	directive: DiscordDirective,
): DiscordActionPlan[] {
	const baseActionId = `${event.eventId}:${directive.directiveType}`;
	const statusText = `alive; ready=${envelope.startedAt}; session=${envelope.sessionId}`;
	const mentionPrefix =
		config.replyMode === "mention-only" && event.authorId.length > 0 ? `<@${event.authorId}> ` : "";
	if (directive.directiveType === "ignore") {
		return [
			{
				actionId: `${baseActionId}:no-op`,
				sourceEventId: event.eventId,
				actionType: "noOp",
				targetChannelId: directive.targetChannelId,
				targetMessageId: event.messageId,
				bodyText: "",
				emoji: "",
			},
		];
	}
	if (directive.directiveType === "ping") {
		const reply = `${mentionPrefix}pong`;
		return config.threadMode === "off"
			? [
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				]
			: [
					{
						actionId: `${baseActionId}:thread`,
						sourceEventId: event.eventId,
						actionType: "openThread",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: "",
						emoji: "",
					},
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				];
	}
	if (directive.directiveType === "status") {
		const reply = `${mentionPrefix}${statusText}`;
		return config.threadMode === "off"
			? [
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				]
			: [
					{
						actionId: `${baseActionId}:thread`,
						sourceEventId: event.eventId,
						actionType: "openThread",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: "",
						emoji: "",
					},
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				];
	}
	if (directive.directiveType === "echo") {
		const reply = `${mentionPrefix}${directive.argumentText}`;
		return config.threadMode === "off"
			? [
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				]
			: [
					{
						actionId: `${baseActionId}:thread`,
						sourceEventId: event.eventId,
						actionType: "openThread",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: "",
						emoji: "",
					},
					{
						actionId: `${baseActionId}:reply`,
						sourceEventId: event.eventId,
						actionType: "reply",
						targetChannelId: directive.targetChannelId,
						targetMessageId: event.messageId,
						bodyText: reply,
						emoji: "",
					},
				];
	}
	return [
		{
			actionId: `${baseActionId}:react`,
			sourceEventId: event.eventId,
			actionType: "addReaction",
			targetChannelId: directive.targetChannelId,
			targetMessageId: event.messageId,
			bodyText: "",
			emoji: directive.argumentText,
		},
	];
}

function isAllowed(value: string, allowlist: readonly string[]): boolean {
	if (allowlist.length === 0) {
		return false;
	}
	return allowlist.includes("*") || allowlist.includes(value);
}

function serializeChannelCursor(state: DiscordTransportState): Record<string, string> {
	try {
		const parsed = JSON.parse(state.lastSeenByChannel) as Record<string, string>;
		return typeof parsed === "object" && parsed !== null ? parsed : {};
	} catch {
		return {};
	}
}

function updateChannelCursor(state: DiscordTransportState, channelId: string, messageId: string): void {
	const current = serializeChannelCursor(state);
	current[channelId] = messageId;
	state.lastSeenByChannel = JSON.stringify(current);
}

function makeNoopClient(): DiscordTransportClientAdapter {
	return {
		async start() {},
		async stop() {},
		onReady() {
			return () => {};
		},
		onMessageCreate() {
			return () => {};
		},
		onReactionAdd() {
			return () => {};
		},
		onError() {
			return () => {};
		},
	};
}

export function createDiscordJsTransportClient(): DiscordTransportClientAdapter {
	const client = new Client({
		intents: [
			GatewayIntentBits.Guilds,
			GatewayIntentBits.GuildMessages,
			GatewayIntentBits.MessageContent,
			GatewayIntentBits.GuildMessageReactions,
		],
	});
	return {
		async start(token: string): Promise<void> {
			await client.login(token);
			if (!client.user) {
				await new Promise<void>((resolve) => client.once("ready", () => resolve()));
			}
		},
		async stop(): Promise<void> {
			client.removeAllListeners();
			await client.destroy();
		},
		onReady(handler: (identity: DiscordClientIdentity) => void): () => void {
			const listener = (): void => {
				handler({ botUserId: client.user?.id ?? "", applicationId: client.application?.id ?? "" });
			};
			client.on("ready", listener);
			return () => client.off("ready", listener);
		},
		onMessageCreate(handler: (message: DiscordTransportMessageLike) => void): () => void {
			const listener = (message: DiscordTransportMessageLike): void => {
				handler(message);
			};
			client.on("messageCreate", listener);
			return () => client.off("messageCreate", listener);
		},
		onReactionAdd(handler: (reaction: DiscordTransportReactionLike, userId: string) => void): () => void {
			const listener = (reaction: DiscordTransportReactionLike, user: { id: string }): void => {
				handler(reaction, user.id);
			};
			client.on("messageReactionAdd", listener);
			return () => client.off("messageReactionAdd", listener);
		},
		onError(handler: (error: Error) => void): () => void {
			const listener = (error: Error): void => handler(error);
			client.on("error", listener);
			return () => client.off("error", listener);
		},
	};
}

export class DiscordTransport {
	private readonly config: ResolvedDiscordTransportConfig;
	private sessionLike: DiscordTransportSessionLike | undefined;
	private readonly client: DiscordTransportClientAdapter;
	private readonly host: string;
	private readonly runtimeVersion: string;
	private readonly packageVersion: string;
	private state: DiscordTransportState | undefined;
	private envelope: DiscordSessionEnvelope | undefined;
	private cleanup: Array<() => void> = [];
	private heartbeatTimer: ReturnType<typeof setInterval> | undefined;
	private started = false;
	private processingEventIds = new Set<string>();

	constructor(options: DiscordTransportOptions) {
		this.config = options.config;
		this.sessionLike = options.session;
		this.client = options.client ?? makeNoopClient();
		this.host = options.host ?? process.env.HOSTNAME ?? process.env.COMPUTERNAME ?? "unknown-host";
		this.runtimeVersion = options.runtimeVersion ?? `${process.release.name}/${process.version}`;
		this.packageVersion = options.packageVersion ?? VERSION;
	}

	setSession(session: DiscordTransportSessionLike): void {
		this.sessionLike = session;
	}

	detachSession(): void {
		this.sessionLike = undefined;
	}

	getEnvelope(): DiscordSessionEnvelope | undefined {
		return this.envelope;
	}

	getState(): DiscordTransportState | undefined {
		return this.state;
	}

	private async persistState(): Promise<void> {
		if (!this.state) {
			return;
		}
		await saveDiscordTransportState(this.config.statePath, this.state);
	}

	private async appendSessionMessage(
		customType: string,
		content: string,
		display: boolean,
		details?: unknown,
	): Promise<void> {
		await this.sessionLike?.sendCustomMessage({ customType, content, display, details });
	}

	private scopeAllows(event: DiscordInboundEvent): boolean {
		if (!this.config.enabled) {
			return false;
		}
		if (event.guildId.length === 0 || event.channelId.length === 0) {
			return false;
		}
		if (!isAllowed(event.guildId, this.config.guildAllowlist)) {
			return false;
		}
		if (!isAllowed(event.channelId, this.config.channelAllowlist)) {
			return false;
		}
		if (
			this.config.ownerAllowlist.length > 0 &&
			event.authorId.length > 0 &&
			!isAllowed(event.authorId, this.config.ownerAllowlist)
		) {
			return false;
		}
		return true;
	}

	private async handleReady(identity: DiscordClientIdentity): Promise<void> {
		this.envelope = createDiscordSessionEnvelope({
			host: this.host,
			runtimeVersion: this.runtimeVersion,
			packageVersion: this.packageVersion,
			botUserId: identity.botUserId,
			applicationId: identity.applicationId,
			transportMode: this.config.dryRun ? "gateway-dry-run" : "gateway",
		});
		this.state ??= {
			lastReadyAt: "",
			lastHeartbeatAt: "",
			processedEventIds: new Set(),
			processedActionIds: new Set(),
			lastSeenByChannel: "{}",
		};
		this.state.lastReadyAt = this.envelope.startedAt;
		this.state.lastHeartbeatAt = this.envelope.startedAt;
		const readyEvent = normalizeDiscordReadyEvent(this.envelope);
		await this.appendSessionMessage("discord.transport.ready", JSON.stringify(readyEvent), true, {
			identity,
			envelope: this.envelope,
		});
		await this.persistState();
		if (this.heartbeatTimer) {
			clearInterval(this.heartbeatTimer);
		}
		this.heartbeatTimer = setInterval(() => {
			void this.handleHeartbeat().catch((error) => this.handleFailure(error));
		}, 30000);
	}

	private async handleHeartbeat(): Promise<void> {
		if (!this.state || !this.envelope) {
			return;
		}
		const heartbeat = createDiscordHeartbeatEvent(this.envelope);
		this.state.lastHeartbeatAt = heartbeat.createdAt;
		await this.appendSessionMessage("discord.transport.heartbeat", JSON.stringify(heartbeat), true);
		await this.persistState();
	}

	private async executePlan(message: DiscordTransportMessageLike, plan: DiscordActionPlan): Promise<void> {
		if (!this.state || !this.envelope) {
			throw new Error("Discord transport is not ready");
		}
		if (this.state.processedActionIds.has(plan.actionId)) {
			return;
		}
		if (this.config.dryRun) {
			await this.appendSessionMessage("discord.transport.action", JSON.stringify(plan), true, { plan });
			this.state.processedActionIds.add(plan.actionId);
			return;
		}

		switch (plan.actionType) {
			case "reply":
				await message.reply(plan.bodyText);
				break;
			case "addReaction":
				if (plan.emoji.length > 0) {
					await message.react(plan.emoji);
				}
				break;
			case "openThread":
			case "logOnly":
			case "noOp":
				break;
		}

		await this.appendSessionMessage("discord.transport.action", JSON.stringify(plan), true, {
			plan,
		});
		this.state.processedActionIds.add(plan.actionId);
	}

	private async handleMessage(message: DiscordTransportMessageLike): Promise<void> {
		if (!this.state || !this.envelope) {
			return;
		}
		if (message.author.bot) {
			return;
		}
		const event = normalizeDiscordMessage(message);
		if (!this.scopeAllows(event)) {
			return;
		}
		if (this.state.processedEventIds.has(event.eventId) || this.processingEventIds.has(event.eventId)) {
			return;
		}
		this.processingEventIds.add(event.eventId);
		try {
			await this.appendSessionMessage("discord.transport.event", JSON.stringify(event), true, {
				event,
			});
			const directive = parseDiscordDirective(
				this.config.commandPrefix,
				event.content,
				event.eventId,
				event.channelId,
			);
			const plans = planDiscordActions(this.config, this.envelope, event, directive);
			const actionablePlans = plans.slice(0, this.config.maxActionsPerTurn);
			if (plans.length > actionablePlans.length) {
				await this.appendSessionMessage(
					"discord.transport.action",
					JSON.stringify({
						truncated: true,
						count: plans.length,
						maxActionsPerTurn: this.config.maxActionsPerTurn,
					}),
					true,
				);
			}

			for (const plan of actionablePlans) {
				if (plan.actionType === "noOp") {
					continue;
				}
				if (plan.actionType === "logOnly") {
					await this.appendSessionMessage("discord.transport.action", JSON.stringify(plan), true, {
						plan,
					});
					continue;
				}
				await this.executePlan(message, plan);
			}

			this.state.processedEventIds.add(event.eventId);
			updateChannelCursor(this.state, event.channelId, event.messageId);
			this.state.lastHeartbeatAt = new Date().toISOString();
			await this.persistState();
		} finally {
			this.processingEventIds.delete(event.eventId);
		}
	}

	private async handleReaction(reaction: DiscordTransportReactionLike, userId: string): Promise<void> {
		if (!this.state || !this.envelope) {
			return;
		}
		const event = normalizeDiscordReaction(reaction, userId);
		if (!this.scopeAllows(event)) {
			return;
		}
		if (this.state.processedEventIds.has(event.eventId) || this.processingEventIds.has(event.eventId)) {
			return;
		}
		this.processingEventIds.add(event.eventId);
		try {
			await this.appendSessionMessage("discord.transport.event", JSON.stringify(event), true, {
				event,
			});
			this.state.processedEventIds.add(event.eventId);
			this.state.lastHeartbeatAt = new Date().toISOString();
			await this.persistState();
		} finally {
			this.processingEventIds.delete(event.eventId);
		}
	}

	private async handleFailure(error: unknown): Promise<void> {
		if (!(error instanceof Error)) {
			throw new Error(String(error));
		}
		if (this.config.failClosed) {
			await this.stop();
			throw error;
		}
		console.error(error);
	}

	async start(): Promise<void> {
		if (!this.config.enabled || this.started) {
			return;
		}
		const token = await resolveDiscordToken(this.config.tokenSource);
		if (!token) {
			throw new Error(`Missing Discord bot token from ${this.config.tokenSource}`);
		}
		await mkdir(dirname(this.config.statePath), { recursive: true });
		this.state = await loadDiscordTransportState(this.config.statePath);
		this.cleanup.push(
			this.client.onError((error) => {
				void this.handleFailure(error);
			}),
		);
		this.cleanup.push(
			this.client.onReady((identity) => {
				void this.handleReady(identity).catch((error) => this.handleFailure(error));
			}),
		);
		this.cleanup.push(
			this.client.onMessageCreate((message) => {
				void this.handleMessage(message).catch((error) => this.handleFailure(error));
			}),
		);
		this.cleanup.push(
			this.client.onReactionAdd((reaction, userId) => {
				void this.handleReaction(reaction, userId).catch((error) => this.handleFailure(error));
			}),
		);
		await this.client.start(token);
		this.started = true;
	}

	async stop(): Promise<void> {
		if (!this.started && !this.envelope && !this.state) {
			return;
		}
		this.started = false;
		if (this.heartbeatTimer) {
			clearInterval(this.heartbeatTimer);
			this.heartbeatTimer = undefined;
		}
		for (const cleanup of this.cleanup.splice(0)) {
			cleanup();
		}
		try {
			await this.persistState();
		} finally {
			await this.client.stop();
		}
	}
}

export function createDiscordTransport(options: DiscordTransportOptions): DiscordTransport {
	return new DiscordTransport(options);
}
