import { spawn } from "node:child_process";
import { createConnection } from "node:net";
import { mkdtempSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { binary as defaultBinary, digest } from "./build.mjs";
import { readFileSync } from "node:fs";

export const port = Number(process.env.AICP_TEST_PORT ?? 7331);
if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error("AICP_TEST_PORT must be an integer from 1 to 65535");
export const baseURL = `http://127.0.0.1:${port}`;
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
export async function assertFreePort() {
  await new Promise((resolve, reject) => {
    const socket = createConnection({ host: "127.0.0.1", port });
    socket.setTimeout(1000);
    socket.once("connect", () => {
      socket.destroy();
      reject(
        new Error(
          `Inconclusive: port ${port} occupied; select a free AICP_TEST_PORT before testing.`,
        ),
      );
    });
    socket.once("error", (error) => {
      socket.destroy();
      if (error.code === "ECONNREFUSED") resolve();
      else reject(error);
    });
    socket.once("timeout", () => {
      socket.destroy();
      reject(new Error("Inconclusive: cannot establish port availability."));
    });
  });
}
export async function stopChild(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill();
  const deadline = Date.now() + 5000;
  while (
    child.exitCode === null &&
    child.signalCode === null &&
    Date.now() < deadline
  )
    await wait(25);
  if (child.exitCode === null && child.signalCode === null) {
    child.kill("SIGKILL");
    throw new Error(
      `Cleanup failed: process ${child.pid} did not exit within five seconds.`,
    );
  }
}

export class OwnedServer {
  constructor({
    binary = process.env.AICP_TEST_BINARY ?? defaultBinary,
    profile = process.env.AICP_TEST_PROFILE ?? "core",
  } = {}) {
    this.binary = path.resolve(binary);
    this.profile = profile;
    this.data = mkdtempSync(path.join(tmpdir(), "aicp-tests-"));
    this.starts = [];
    this.logs = [];
    this.children = new Set();
  }
  environment() {
    const env = { ...process.env };
    env.AICP_SERVER_URL = baseURL;
    env.AICP_DATA_DIR = this.data;
    delete env.AICP_PLANTUML_JAR;
    delete env.AICP_TEST_PLANTUML_JAR;
    if (this.profile === "renderer")
      env.AICP_PLANTUML_JAR = process.env.AICP_TEST_PLANTUML_JAR;
    return env;
  }
  async start() {
    await assertFreePort();
    let log = "";
    const child = spawn(this.binary, ["serve", "--data-dir", this.data, "--port", String(port)], {
      env: this.environment(),
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    this.child = child;
    this.children.add(child);
    let spawnError;
    child.on("error", (error) => {
      spawnError = error;
    });
    child.stdout.on("data", (chunk) => {
      log = (log + chunk).slice(-65536);
    });
    child.stderr.on("data", (chunk) => {
      log = (log + chunk).slice(-65536);
    });
    this.logs.push(() => log);
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      if (spawnError) throw spawnError;
      if (child.exitCode !== null || child.signalCode !== null)
        throw new Error(`Owned server exited: ${log}`);
      // The child's ready message is emitted only after it owns the listener.
      // A stale health endpoint alone cannot satisfy this handshake.
      if (log.includes("aicp is ready")) {
        const response = await fetch(`${baseURL}/healthz`, {
          signal: AbortSignal.timeout(1000),
        });
        if (response.ok && child.exitCode === null) {
          this.starts.push({
            pid: child.pid,
            binary_digest: digest(readFileSync(this.binary)),
            data_directory: this.data,
            started_at: new Date().toISOString(),
            health: await response.json(),
          });
          return;
        }
      }
      await wait(25);
    }
    throw new Error(`Owned server readiness timeout: ${log}`);
  }
  async restart() {
    const previous = this.child.pid;
    await stopChild(this.child);
    await this.start();
    if (this.child.pid === previous)
      throw new Error("Restart did not establish a fresh PID.");
  }
  async close({ preserve = false, diagnostics } = {}) {
    const failures = [];
    for (const child of this.children) {
      try {
        await stopChild(child);
      } catch (error) {
        failures.push(error.message);
      }
    }
    if (this.starts.length && !failures.length) {
      try {
        await assertFreePort();
      } catch (error) {
        failures.push(error.message);
      }
    }
    if (diagnostics) {
      mkdirSync(diagnostics, { recursive: true });
      writeFileSync(
        path.join(diagnostics, "processes.json"),
        JSON.stringify(
          {
            starts: this.starts,
            children: [...this.children].map((child) => ({
              pid: child.pid,
              arguments: child.spawnargs.slice(1),
              exit_code: child.exitCode,
              signal: child.signalCode,
            })),
            cleanup: failures.length ? failures : "passed",
            retained_data: preserve || failures.length ? this.data : null,
          },
          null,
          2,
        ),
      );
      this.logs.forEach((getLog, i) =>
        writeFileSync(path.join(diagnostics, `server-${i + 1}.log`), getLog()),
      );
    }
    if (!preserve && !failures.length) {
      const parent = path.resolve(tmpdir());
      if (
        path.dirname(path.resolve(this.data)) !== parent ||
        !path.basename(this.data).startsWith("aicp-tests-")
      )
        throw new Error(`Unsafe cleanup path: ${this.data}`);
      rmSync(this.data, { recursive: true, force: true });
    }
    if (failures.length)
      throw new Error(`Cleanup failed (${this.data}): ${failures.join("; ")}`);
  }
}

export class MCPClient {
  constructor(server) {
    this.pending = new Map();
    this.nextID = 0;
    this.raw = [];
    this.child = spawn(server.binary, ["mcp", "--server", baseURL], {
      env: server.environment(),
      windowsHide: true,
      stdio: ["pipe", "pipe", "pipe"],
    });
    server.children.add(this.child);
    let buffer = "";
    this.child.stdout.on("data", (chunk) => {
      buffer += chunk;
      for (;;) {
        const newline = buffer.indexOf("\n");
        if (newline < 0) break;
        const line = buffer.slice(0, newline);
        buffer = buffer.slice(newline + 1);
        try {
          const message = JSON.parse(line);
          this.raw.push(message);
          const pending = this.pending.get(message.id);
          if (pending) {
            this.pending.delete(message.id);
            clearTimeout(pending.timer);
            message.error
              ? pending.reject(new Error(JSON.stringify(message.error)))
              : pending.resolve(message.result);
          }
        } catch (error) {
          this.fail(error);
        }
      }
    });
    this.child.on("error", (error) => this.fail(error));
    this.child.on("exit", () => this.fail(new Error("MCP process exited.")));
    this.child.stderr.on("data", (chunk) => {
      this.stderr = ((this.stderr ?? "") + chunk).slice(-65536);
    });
  }
  fail(error) {
    for (const entry of this.pending.values()) {
      clearTimeout(entry.timer);
      entry.reject(error);
    }
    this.pending.clear();
  }
  async request(method, params) {
    const id = ++this.nextID;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`MCP ${method} timed out.`));
      }, 10_000);
      this.pending.set(id, { resolve, reject, timer });
      const message = { jsonrpc: "2.0", id, method, params };
      this.raw.push(message);
      this.child.stdin.write(JSON.stringify(message) + "\n", (error) => {
        if (error) this.fail(error);
      });
    });
  }
  async connect() {
    await this.request("initialize", {
      protocolVersion: "2025-03-26",
      capabilities: {},
      clientInfo: { name: "aicp-journey", version: "1" },
    });
    this.child.stdin.write(
      JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }) +
        "\n",
    );
    return this;
  }
  async call(name, args = {}) {
    const result = await this.request("tools/call", { name, arguments: args });
    if (result.isError)
      throw new Error(`MCP ${name}: ${JSON.stringify(result)}`);
    return (
      result.structuredContent ??
      JSON.parse(result.content.find((c) => c.type === "text").text)
    );
  }
  async close() {
    this.fail(new Error("MCP session closed."));
    await stopChild(this.child);
  }
}
