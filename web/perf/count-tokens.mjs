// Receives synthetic MCP results or exact CLI stdout on stdin; emits counts without retaining it.
import { getEncoding } from "js-tiktoken";

let raw = "";
for await (const chunk of process.stdin) raw += chunk;
const input = JSON.parse(raw);
const encoding = getEncoding("o200k_base");
const observations = input.map(({ category, text }) => ({
  category,
  bytes: Buffer.byteLength(text),
  tokens: encoding.encode(text).length,
}));
process.stdout.write(JSON.stringify({ tokenizer: "js-tiktoken@1.0.21/o200k_base", observations }));
