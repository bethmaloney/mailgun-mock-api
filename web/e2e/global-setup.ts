import { execSync, spawn, type ChildProcess } from "child_process";
import { writeFileSync } from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = 8026;
const ROOT = path.resolve(__dirname, "../..");
const PID_FILE = path.join(__dirname, ".server.pid");

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

  // Start the server with in-memory SQLite on a test port
  console.log(`Starting server on port ${PORT}...`);
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
      stdio: "pipe",
    },
  );

  server.stderr?.on("data", (data: Buffer) => {
    process.stderr.write(`[server] ${data}`);
  });

  // Store PID so global-teardown can kill it
  writeFileSync(PID_FILE, String(server.pid));

  await waitForHealthy(`http://localhost:${PORT}/mock/health`);
  console.log("Server is ready.");
}
