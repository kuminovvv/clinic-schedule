import {spawn} from "node:child_process"
import {readFileSync} from "node:fs"
import {resolve} from "node:path"

const projectRoot = process.cwd()
const envFile = resolve(projectRoot, ".env.local")
try {
  const source = readFileSync(envFile, "utf8").replace(/^\uFEFF/, "")
  for (const rawLine of source.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith("#")) continue
    const separator = line.indexOf("=")
    if (separator < 1) continue

    const name = line.slice(0, separator).trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) || process.env[name] !== undefined) continue
    let value = line.slice(separator + 1).trim()
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    process.env[name] = value
  }
} catch (error) {
  if (error.code !== "ENOENT") throw error
}

const server = spawn(process.execPath, [resolve(projectRoot, ".next", "standalone", "server.js")], {
  cwd: projectRoot,
  env: process.env,
  stdio: "inherit",
})

server.on("error", (error) => {
  console.error("Could not start the standalone Next.js server:", error)
  process.exitCode = 1
})

server.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal)
  else process.exitCode = code ?? 1
})
