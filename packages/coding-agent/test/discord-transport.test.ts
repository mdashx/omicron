import { existsSync, mkdirSync, readFileSync, rmSync } from "fs";
import { writeFile } from "fs/promises";
import { join } from "path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { AgentSession } from "../src/core/agent-session.ts";
import {
	createDiscordTransport,
	type DiscordClientIdentity,
	type DiscordInboundEvent,
	type DiscordTransportClientAdapter,
	type DiscordTransportMessageLike,
	type DiscordTransportReactionLike,
	type DiscordTransportSessionLike,
	parseDiscordDirective,
	planDiscordActions,
	resolveDiscordToken,
	resolveDiscordTransportConfig,
} from "../src/core/discord-transport.ts";

class FakeDiscordClient implements DiscordTransportClientAdapter {
	private readyHandlers = new Set<(identity: DiscordClientIdentity) => void>();
	private messageHandlers = new Set<(message: DiscordTransportMessageLike) => void>();
	private reactionHandlers = new Set<(reaction: DiscordTransportReactionLike, userId: string) => void>();
	private errorHandlers = new Set<(error: Error) => void>();
	startedToken = "";
	stopped = false;

	async start(token: string): Promise<void> {
		this.startedToken = token;
		for (const handler of this.readyHandlers) {
			handler({ botUserId: "bot-123", applicationId: "app-123" });
		}
	}

	async stop(): Promise<void> {
		this.stopped = true;
	}

	onReady(handler: (identity: DiscordClientIdentity) => void): () => void {
		this.readyHandlers.add(handler);
		return () => this.readyHandlers.delete(handler);
	}

	onMessageCreate(handler: (message: DiscordTransportMessageLike) => void): () => void {
		this.messageHandlers.add(handler);
		return () => this.messageHandlers.delete(handler);
	}

	onReactionAdd(handler: (reaction: DiscordTransportReactionLike, userId: string) => void): () => void {
		this.reactionHandlers.add(handler);
		return () => this.reactionHandlers.delete(handler);
	}

	onError(handler: (error: Error) => void): () => void {
		this.errorHandlers.add(handler);
		return () => this.errorHandlers.delete(handler);
	}

	emitMessage(message: DiscordTransportMessageLike): void {
		for (const handler of this.messageHandlers) {
			handler(message);
		}
	}
}

class FakeSession implements DiscordTransportSessionLike {
	readonly messages: Array<{ customType: string; content: unknown; display: boolean; details?: unknown }> = [];

	sendCustomMessage: AgentSession["sendCustomMessage"] = async (...args) => {
		const [message] = args;
		this.messages.push(message);
	};
}

describe("discord transport", () => {
	const testDir = join(process.cwd(), "test-discord-transport-tmp");
	const statePath = join(testDir, "state", "state.json");
	const originalToken = process.env.TEST_DISCORD_TOKEN;

	beforeEach(() => {
		if (existsSync(testDir)) {
			rmSync(testDir, { recursive: true });
		}
		mkdirSync(testDir, { recursive: true });
		process.env.TEST_DISCORD_TOKEN = "discord-token";
	});

	afterEach(() => {
		if (originalToken === undefined) {
			delete process.env.TEST_DISCORD_TOKEN;
		} else {
			process.env.TEST_DISCORD_TOKEN = originalToken;
		}
		if (existsSync(testDir)) {
			rmSync(testDir, { recursive: true });
		}
	});

	it("resolves config overrides and env values", () => {
		const resolved = resolveDiscordTransportConfig(
			{
				statePath,
				guildAllowlist: ["guild-a"],
				channelAllowlist: ["channel-a"],
				tokenSource: "env.TEST_DISCORD_TOKEN",
				maxActionsPerTurn: 4,
			},
			{
				PI_DISCORD_DRY_RUN: "false",
				PI_DISCORD_REPLY_MODE: "mention-only",
			},
		);

		expect(resolved.enabled).toBe(true);
		expect(resolved.dryRun).toBe(false);
		expect(resolved.replyMode).toBe("mention-only");
		expect(resolved.guildAllowlist).toEqual(["guild-a"]);
		expect(resolved.channelAllowlist).toEqual(["channel-a"]);
		expect(resolved.statePath).toBe(statePath);
		expect(resolved.maxActionsPerTurn).toBe(4);
	});

	it("resolves file token sources", async () => {
		const tokenPath = join(testDir, "token.txt");
		await writeFile(tokenPath, "file-token\n", "utf8");
		await expect(resolveDiscordToken(`file:${tokenPath}`)).resolves.toBe("file-token");
	});

	it("parses directives and plans deterministic actions", () => {
		const directive = parseDiscordDirective("!", "!echo hello world", "evt-1", "channel-1");
		expect(directive).toEqual({
			directiveType: "echo",
			sourceEventId: "evt-1",
			targetChannelId: "channel-1",
			argumentText: "hello world",
		});

		const config = resolveDiscordTransportConfig(
			{
				statePath,
				guildAllowlist: ["guild-1"],
				channelAllowlist: ["channel-1"],
				tokenSource: "env.TEST_DISCORD_TOKEN",
				dryRun: true,
				threadMode: "off",
				replyMode: "reply-to-message",
			},
			{},
		);
		const event: DiscordInboundEvent = {
			eventId: "evt-1",
			eventType: "messageCreate",
			guildId: "guild-1",
			channelId: "channel-1",
			messageId: "msg-1",
			authorId: "user-1",
			content: "!echo hello world",
			createdAt: new Date().toISOString(),
			jumpUrl: "https://discord.com/channels/1/2/3",
			rawKind: "gateway.messageCreate",
		};
		const envelope = {
			sessionId: "session-1",
			startedAt: new Date().toISOString(),
			source: "pi-discord-transport",
			host: "host",
			runtimeVersion: "node/v22",
			packageVersion: "0.0.0",
			botUserId: "bot-123",
			applicationId: "app-123",
			transportMode: "gateway-dry-run",
		};

		expect(planDiscordActions(config, envelope, event, directive)).toEqual([
			{
				actionId: "evt-1:echo:reply",
				sourceEventId: "evt-1",
				actionType: "reply",
				targetChannelId: "channel-1",
				targetMessageId: "msg-1",
				bodyText: "hello world",
				emoji: "",
			},
		]);
	});

	it("deduplicates processed events and persists state", async () => {
		const client = new FakeDiscordClient();
		const session = new FakeSession();
		const config = resolveDiscordTransportConfig(
			{
				statePath,
				guildAllowlist: ["guild-1"],
				channelAllowlist: ["channel-1"],
				tokenSource: "env.TEST_DISCORD_TOKEN",
				dryRun: true,
			},
			{},
		);
		const transport = createDiscordTransport({
			config,
			session,
			client,
			host: "host",
			runtimeVersion: "node/v22",
			packageVersion: "0.0.0",
		});

		await transport.start();
		await Promise.resolve();

		client.emitMessage({
			id: "msg-1",
			channelId: "channel-1",
			guildId: "guild-1",
			author: { id: "user-1", bot: false },
			content: "!ping",
			createdTimestamp: Date.now(),
			url: "https://discord.com/channels/1/2/3",
			reply: async () => undefined,
			react: async () => undefined,
		});
		client.emitMessage({
			id: "msg-1",
			channelId: "channel-1",
			guildId: "guild-1",
			author: { id: "user-1", bot: false },
			content: "!ping",
			createdTimestamp: Date.now(),
			url: "https://discord.com/channels/1/2/3",
			reply: async () => undefined,
			react: async () => undefined,
		});

		await Promise.resolve();
		await transport.stop();

		expect(client.startedToken).toBe("discord-token");
		expect(session.messages.some((message) => message.customType === "discord.transport.ready")).toBe(true);
		expect(session.messages.filter((message) => message.customType === "discord.transport.event")).toHaveLength(1);
		expect(session.messages.filter((message) => message.customType === "discord.transport.action")).toHaveLength(1);
		expect(JSON.parse(readFileSync(statePath, "utf8"))).toMatchObject({
			processedEventIds: ["msg_msg-1"],
			processedActionIds: ["msg_msg-1:ping:reply"],
		});
	});
});
