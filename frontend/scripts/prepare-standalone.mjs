import {cpSync, existsSync, rmSync} from "node:fs"
import {join} from "node:path"

const projectRoot = process.cwd()
const standaloneRoot = join(projectRoot, ".next", "standalone")
const staticSource = join(projectRoot, ".next", "static")
const staticTarget = join(standaloneRoot, ".next", "static")

if (!existsSync(join(standaloneRoot, "server.js")) || !existsSync(staticSource)) {
  throw new Error("Run `next build` before preparing the standalone server.")
}

rmSync(staticTarget, {recursive: true, force: true})
cpSync(staticSource, staticTarget, {recursive: true})

const publicSource = join(projectRoot, "public")
const publicTarget = join(standaloneRoot, "public")
if (existsSync(publicSource)) {
  rmSync(publicTarget, {recursive: true, force: true})
  cpSync(publicSource, publicTarget, {recursive: true})
}
