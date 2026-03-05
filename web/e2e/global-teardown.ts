import { readFileSync, unlinkSync } from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PID_FILE = path.join(__dirname, ".server.pid");

export default async function globalTeardown() {
  try {
    const pid = parseInt(readFileSync(PID_FILE, "utf-8").trim(), 10);
    console.log(`Stopping server (pid ${pid})...`);
    process.kill(pid, "SIGTERM");
  } catch {
    // server may have already exited
  }
  try {
    unlinkSync(PID_FILE);
  } catch {
    // file may not exist
  }
}
