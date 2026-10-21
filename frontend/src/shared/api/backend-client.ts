import {explainApiError} from "@/shared/api/error-messages"
import {notify} from "@/shared/lib/notifications"

type RequestMethod = "GET" | "POST" | "PUT" | "DELETE"

export class BackendRequestError extends Error {
  constructor(
    readonly status: number,
    readonly reason: string,
    readonly path: string,
  ) {
    super(`Backend request failed: ${status} at ${path}: ${reason}`)
    this.name = "BackendRequestError"
  }
}

const getErrorReason = async (response: Response): Promise<string> => {
  try {
    const content = await response.json() as {error?: unknown}
    return typeof content.error === "string" ? content.error : ""
  } catch {
    return ""
  }
}

export const requestBackend = async <T>(
  path: string,
  method: RequestMethod = "GET",
  body?: unknown,
): Promise<T | null> => {
  try {
    const response = await fetch("/api/backend" + path, {
      method,
      headers: body === undefined ? undefined : {"Content-Type": "application/json"},
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    })
    if (!response.ok) throw new BackendRequestError(response.status, await getErrorReason(response), path)
    if (response.status === 204) return null
    try {
      return await response.json() as T
    } catch {
      throw new BackendRequestError(response.status, "invalid response body", path)
    }
  } catch (error) {
    const issue = error instanceof BackendRequestError
      ? {status: error.status, reason: error.reason, path, method}
      : {path, method}
    const notice = explainApiError(issue)
    if (notice.type === "warning") notify.warning(notice.title, notice.description)
    else notify.error(notice.title, notice.description)
    throw error
  }
}

export const getClientData = async <T>(path: string): Promise<T> => {
  const data = await requestBackend<T>(path)
  return data as T
}

export const sendBackendRequest = <T>(
  path: string, method: Exclude<RequestMethod, "GET">, body?: unknown,
): Promise<T | null> => requestBackend<T>(path, method, body)
