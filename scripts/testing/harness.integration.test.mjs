import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { root } from "./build.mjs";
import { run } from "./process.mjs";
import { createConnection } from "node:net";

test("real Go source discovery refuses unregistered runnable examples and fuzz families", async () => {
  const directory = mkdtempSync(path.join(tmpdir(), "aicp-discovery-control-"));
  try {
    for (const name of ["ExampleLost", "FuzzLost"]) {
      writeFileSync(
        path.join(directory, "control_test.go"),
        `package control\nfunc ${name}() {}\n`,
      );
      await assert.rejects(
        run("go", ["run", path.join(root, "scripts/testing/discover.go")], {
          cwd: directory,
        }),
        /unsupported runnable Go family/,
      );
    }
  } finally {
    rmSync(directory, { recursive: true });
  }
});
test("command timeout terminates its owned descendant listener and returns within a bound", async () => {
  const childSource =
    'require("node:net").createServer().listen(0,"127.0.0.1",function(){console.log("PORT:"+this.address().port)})';
  const parentSource = `require("node:child_process").spawn(process.execPath,["-e",${JSON.stringify(childSource)}],{stdio:["ignore","inherit","inherit"]});setInterval(()=>{},1000)`;
  let result;
  try {
    await run(process.execPath, ["-e", parentSource], { timeout: 1000 });
    assert.fail("Timed-out command unexpectedly passed");
  } catch (error) {
    assert.equal(error.result?.timedOut, true);
    result = error.result;
  }
  const port = Number(result.stdout.match(/PORT:(\d+)/)?.[1]);
  assert.ok(port, "Owned child must have started before timeout");
  await new Promise((resolve, reject) => {
    const socket = createConnection({ host: "127.0.0.1", port });
    socket.once("connect", () => {
      socket.destroy();
      reject(new Error("Owned descendant survived timeout"));
    });
    socket.once("error", (error) => {
      socket.destroy();
      error.code === "ECONNREFUSED" ? resolve() : reject(error);
    });
    socket.setTimeout(1000, () => {
      socket.destroy();
      reject(new Error("Port cleanup observation timed out"));
    });
  });
});
