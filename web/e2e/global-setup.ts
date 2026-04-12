import { execSync, spawn, type ChildProcess } from "child_process";
import { openSync, writeFileSync } from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = 8026;
const ROOT = path.resolve(__dirname, "../..");
const PID_FILE = path.join(__dirname, ".server.pid");
const LOG_FILE = path.join(__dirname, ".server.log");

async function waitForHealthy(
  url: string,
  timeoutMs = 15000,
): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`Server did not become healthy at ${url} within ${timeoutMs}ms`);
}

export default async function globalSetup() {
  // Build the binary (Vue SPA + Go)
  console.log("Building project...");
  execSync("just build", { cwd: ROOT, stdio: "inherit" });

  // Start the server with in-memory SQLite on a test port.
  //
  // IMPORTANT: server stdout/stderr are redirected to a file on disk,
  // NOT piped back to this Node process. The previous "stdio: pipe" +
  // data-event-listener approach deadlocked the entire suite reliably:
  // the server's chi middleware.Logger and GORM's query logger both
  // write to stderr, and a 64 KB Linux pipe buffer fills in ~300
  // requests. Once full, os.File.Write blocks inside the kernel write
  // syscall while holding Go's fd-mutex, wedging every HTTP handler
  // behind it. Node's event loop, busy running Playwright, couldn't
  // drain the pipe fast enough to unstick it.
  //
  // Writing to a file sidesteps the problem entirely: file writes don't
  // backpressure the way pipes do (kernel buffers + page cache absorb
  // bursts), and Node doesn't have to participate at all. Server logs
  // are still inspectable after a failure — see .server.log.
  console.log(`Starting server on port ${PORT}...`);
  console.log(`Server logs → ${LOG_FILE}`);
  const logFd = openSync(LOG_FILE, "w");
  const server: ChildProcess = spawn(
    "./mailgun-mock-api",
    [],
    {
      cwd: ROOT,
      env: {
        ...process.env,
        PORT: String(PORT),
        DATABASE_URL: "file::memory:?cache=shared",
        DB_DRIVER: "sqlite",
      },
      stdio: ["ignore", logFd, logFd],
    },
  );

  // Store PID so global-teardown can kill it
  writeFileSync(PID_FILE, String(server.pid));

  await waitForHealthy(`http://localhost:${PORT}/mock/health`);
  console.log("Server is ready.");
}
