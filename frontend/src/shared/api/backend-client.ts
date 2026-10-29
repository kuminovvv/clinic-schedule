import {explainApiError} from "@/shared/api/error-messages"
import {notify} from "@/shared/lib/notifications"
import type {UnscheduledDetail} from "@/shared/api/schedule-feedback"

type RequestMethod = "GET" | "POST" | "PUT" | "DELETE"

export class BackendRequestError extends Error {
  constructor(
    readonly status: number,
    readonly reason: string,
    readonly path: string,
    readonly unscheduledDetails: UnscheduledDetail[] = [],
    readonly unscheduledCount = 0,
  ) {
    super(`Backend request failed: ${status} at ${path}: ${reason}`)
    this.name = "BackendRequestError"
  }
}

const getErrorDetails = async (response: Response) => {
  try {
    const content = await response.json() as {error?: unknown; unscheduledDetails?: unknown; unscheduledCount?: unknown}
    const details: UnscheduledDetail[] = Array.isArray(content.unscheduledDetails) ? content.unscheduledDetails.filter((item): item is UnscheduledDetail =>
      item !== null && typeof item === "object" && Number.isInteger(item.patientNumber) &&
      typeof item.procedureName === "string" && Number.isInteger(item.requestedCount) && Number.isInteger(item.missingCount),
    ) : []
    return {reason: typeof content.error === "string" ? content.error : "", details,
      count: typeof content.unscheduledCount === "number" ? content.unscheduledCount : 0}
  } catch {
    return {reason: "", details: [], count: 0}
  }
}

export const requestBackend = async <T>(
  path: string,
  method: RequestMethod = "GET",
  body?: unknown,
  options?: {headers?: Record<string, string>; onResponse?: (response: Response) => void},
): Promise<T | null> => {
  try {
    const response = await fetch("/api/backend" + path, {
      method,
      headers: { ...(body === undefined ? {} : {"Content-Type": "application/json"}), ...options?.headers },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    })
    if (!response.ok) {
      const {reason, details, count} = await getErrorDetails(response)
      throw new BackendRequestError(response.status, reason, path, details, count)
    }
    options?.onResponse?.(response)
    if (response.status === 204) return null
    try {
      return await response.json() as T
    } catch {
      throw new BackendRequestError(response.status, "invalid response body", path)
    }
  } catch (error) {
    const issue = error instanceof BackendRequestError
      ? {status: error.status, reason: error.reason, path, method, unscheduledDetails: error.unscheduledDetails, unscheduledCount: error.unscheduledCount}
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
  options?: {headers?: Record<string, string>; onResponse?: (response: Response) => void},
): Promise<T | null> => requestBackend<T>(path, method, body, options)
