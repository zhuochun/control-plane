import { spawn } from "node:child_process";
import path from "node:path";

export async function terminateTree(child) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  if (process.platform === "win32") {
    await new Promise((resolve, reject) => {
      const killer = spawn(
        path.join(process.env.SystemRoot, "System32/taskkill.exe"),
        ["/PID", String(child.pid), "/T", "/F"],
        { windowsHide: true, stdio: "ignore" },
      );
      const timer = setTimeout(() => {
        killer.kill();
        reject(new Error(`Process-tree cleanup timeout: ${child.pid}`));
      }, 5000);
      killer.on("error", (error) => {
        clearTimeout(timer);
        reject(error);
      });
      killer.on("exit", (code) => {
        clearTimeout(timer);
        code === 0 || child.exitCode !== null
          ? resolve()
          : reject(new Error(`Process-tree cleanup failed: ${child.pid}`));
      });
    });
  } else {
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch (error) {
      if (error.code !== "ESRCH") throw error;
    }
  }
}

// No shell interpolation; every argument stays a distinct argument on Windows too.
export function run(
  command,
  args,
  { cwd, env = process.env, timeout = 300_000, echo = false } = {},
) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd,
      env,
      windowsHide: true,
      detached: process.platform !== "win32",
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "",
      stderr = "",
      timedOut = false,
      settled = false;
    const finish = (code, cleanupError) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      const result = { code, stdout, stderr, timedOut, cleanupError };
      if (code !== 0 || timedOut || cleanupError)
        reject(
          Object.assign(
            new Error(
              `${command} failed${timedOut ? " (timeout)" : ""}${cleanupError ? ` (${cleanupError})` : ""}: ${stderr}\n${stdout}`,
            ),
            { result },
          ),
        );
      else resolve(result);
    };
    child.stdout.on("data", (data) => {
      stdout += data;
      if (echo) process.stdout.write(data);
    });
    child.stderr.on("data", (data) => {
      stderr += data;
      if (echo) process.stderr.write(data);
    });
    const timer = setTimeout(async () => {
      timedOut = true;
      let cleanupError;
      try {
        await terminateTree(child);
      } catch (error) {
        cleanupError = error.message;
      }
      child.stdout.destroy();
      child.stderr.destroy();
      child.unref();
      finish(null, cleanupError);
    }, timeout);
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("close", (code) => finish(code));
  });
}
