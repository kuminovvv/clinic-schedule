import type { NextRequest } from "next/server"

const backendUrl = process.env.BACKEND_INTERNAL_URL ?? "http://127.0.0.1:8080"

const forward = async (request: NextRequest, context: { params: Promise<{ path: string[] }> }) => {
  const { path } = await context.params
  const target = new URL("/" + path.join("/"), backendUrl)
  target.search = request.nextUrl.search
  const response = await fetch(target, {
    method: request.method,
    headers: request.headers.get("content-type") ? { "Content-Type": request.headers.get("content-type")! } : undefined,
    body: request.method === "GET" || request.method === "HEAD" ? undefined : await request.arrayBuffer(),
    cache: "no-store",
  })
  return new Response(response.body, { status: response.status, headers: { "Cache-Control": "no-store", "Content-Type": response.headers.get("content-type") ?? "application/json" } })
}

export const GET = forward
export const POST = forward
export const PUT = forward
export const PATCH = forward
export const DELETE = forward
