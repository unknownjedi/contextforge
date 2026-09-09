import type {
  AuthResponse,
  ChatCompletionRequest,
  ChatCompletionResponse,
  Citation,
  CreateProjectRequest,
  CreateSourceRequest,
  DocumentDetail,
  DocumentListResponse,
  IngestionJob,
  ProblemDetails,
  Project,
  ProjectListResponse,
  Source,
  SourceIngestionQueued,
  UpdateProjectRequest,
  User,
  DatabaseSource,
  CreateDatabaseSourceRequest,
  UpdateDatabaseSourceRequest,
  ConnectionTestResult,
  DatabaseMetadata,
} from "@/types/api";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080/api/v1";

export class ApiError extends Error {
  status: number;
  problem?: ProblemDetails;

  constructor(status: number, message: string, problem?: ProblemDetails) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

/**
 * Generic type-safe fetch wrapper configured for /api/v1
 */
export async function apiFetch<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const url = endpoint.startsWith("http")
    ? endpoint
    : `${API_BASE_URL}${endpoint.startsWith("/") ? "" : "/"}${endpoint}`;

  const headers = new Headers(options.headers || {});
  if (!headers.has("Content-Type") && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  if (!headers.has("Accept")) {
    headers.set("Accept", "application/json");
  }

  if (typeof window !== "undefined" && !headers.has("Authorization")) {
    const token = localStorage.getItem("cf_token");
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
  }

  const config: RequestInit = {
    ...options,
    headers,
    credentials: "include",
  };

  const response = await fetch(url, config);

  if (response.status === 204) {
    return null as unknown as T;
  }

  const contentType = response.headers.get("content-type") || "";
  const isJson = contentType.includes("application/json") || contentType.includes("application/problem+json");

  if (!response.ok) {
    let errorDetail: ProblemDetails | undefined;
    let errorMessage = `HTTP Error ${response.status}: ${response.statusText}`;

    if (isJson) {
      try {
        const body = await response.json();
        errorDetail = body as ProblemDetails;
        if (errorDetail.detail) {
          errorMessage = errorDetail.detail;
        } else if (errorDetail.title) {
          errorMessage = errorDetail.title;
        }
      } catch {
        // Fall back to default message
      }
    } else {
      try {
        const text = await response.text();
        if (text) errorMessage = text;
      } catch {
        // Fall back to default message
      }
    }

    throw new ApiError(response.status, errorMessage, errorDetail);
  }

  if (isJson) {
    return (await response.json()) as T;
  }

  return (await response.text()) as unknown as T;
}

// ---------------------------------------------------------------------------
// Authentication API
// ---------------------------------------------------------------------------

export async function getCurrentUser(): Promise<User> {
  return apiFetch<User>("/auth/me");
}

export async function loginWithPat(pat: string): Promise<AuthResponse> {
  const res = await apiFetch<AuthResponse>("/auth/pat", {
    method: "POST",
    body: JSON.stringify({ pat }),
  });
  if (typeof window !== "undefined" && res?.token) {
    localStorage.setItem("cf_token", res.token);
    localStorage.setItem("cf_pat", pat);
  }
  return res;
}

export interface AutoAuthResponse {
  configured: boolean;
  token?: string;
  user?: User;
  message?: string;
}

export async function autoLoginDev(): Promise<AutoAuthResponse | null> {
  try {
    const res = await apiFetch<AutoAuthResponse>("/auth/auto");
    if (typeof window !== "undefined" && res?.token) {
      localStorage.setItem("cf_token", res.token);
    }
    return res;
  } catch {
    return null;
  }
}

export async function logout(): Promise<void> {
  if (typeof window !== "undefined") {
    localStorage.removeItem("cf_token");
    localStorage.removeItem("cf_pat");
  }
  return apiFetch<void>("/auth/logout", {
    method: "POST",
  });
}

// ---------------------------------------------------------------------------
// Projects API
// ---------------------------------------------------------------------------

export async function getProjects(params?: {
  page?: number;
  page_size?: number;
}): Promise<ProjectListResponse> {
  const query = new URLSearchParams();
  if (params?.page) query.set("page", params.page.toString());
  if (params?.page_size) query.set("page_size", params.page_size.toString());
  const qs = query.toString();
  return apiFetch<ProjectListResponse>(`/projects${qs ? `?${qs}` : ""}`);
}

export async function getProject(id: string): Promise<Project> {
  return apiFetch<Project>(`/projects/${id}`);
}

export async function createProject(
  data: CreateProjectRequest
): Promise<Project> {
  return apiFetch<Project>("/projects", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function updateProject(
  id: string,
  data: UpdateProjectRequest
): Promise<Project> {
  return apiFetch<Project>(`/projects/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function deleteProject(id: string): Promise<void> {
  return apiFetch<void>(`/projects/${id}`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Sources API
// ---------------------------------------------------------------------------

export async function getSources(projectId: string): Promise<Source[]> {
  return apiFetch<Source[]>(`/projects/${projectId}/sources`);
}

export async function createSource(
  projectId: string,
  data: CreateSourceRequest
): Promise<SourceIngestionQueued> {
  return apiFetch<SourceIngestionQueued>(`/projects/${projectId}/sources`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function syncSource(
  projectId: string,
  sourceId: string,
  forceFull = false
): Promise<IngestionJob> {
  return apiFetch<IngestionJob>(
    `/projects/${projectId}/sources/${sourceId}/sync`,
    {
      method: "POST",
      body: JSON.stringify({ force_full: forceFull }),
    }
  );
}

// ---------------------------------------------------------------------------
// Documents API
// ---------------------------------------------------------------------------

export async function getDocuments(
  projectId: string,
  params?: { file_path?: string; page?: number; page_size?: number }
): Promise<DocumentListResponse> {
  const query = new URLSearchParams();
  if (params?.file_path) query.set("file_path", params.file_path);
  if (params?.page) query.set("page", params.page.toString());
  if (params?.page_size) query.set("page_size", params.page_size.toString());
  const qs = query.toString();
  return apiFetch<DocumentListResponse>(
    `/projects/${projectId}/documents${qs ? `?${qs}` : ""}`
  );
}

export async function getDocument(
  projectId: string,
  documentId: string
): Promise<DocumentDetail> {
  return apiFetch<DocumentDetail>(
    `/projects/${projectId}/documents/${documentId}`
  );
}

export async function deleteSource(
  projectId: string,
  sourceId: string
): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/sources/${sourceId}`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Ingestion Jobs API
// ---------------------------------------------------------------------------

export async function getJob(
  id: string,
  projectId?: string
): Promise<IngestionJob> {
  try {
    return await apiFetch<IngestionJob>(`/jobs/${id}`);
  } catch (err: any) {
    if (projectId) {
      return await apiFetch<IngestionJob>(`/projects/${projectId}/jobs/${id}`);
    }
    throw err;
  }
}

// ---------------------------------------------------------------------------
// Chat & SSE Streaming API
// ---------------------------------------------------------------------------

export async function chatCompletion(
  projectId: string,
  req: ChatCompletionRequest
): Promise<ChatCompletionResponse> {
  return apiFetch<ChatCompletionResponse>(
    `/projects/${projectId}/chat/completions`,
    {
      method: "POST",
      body: JSON.stringify(req),
    }
  );
}

export interface StreamChatCallbacks {
  onCitation: (citation: Citation) => void;
  onToken: (token: string) => void;
  onDone: (data: { total_tokens: number; duration_ms: number }) => void;
  onError: (error: any) => void;
}

/**
 * Helper for Server-Sent Events (SSE) chat streaming
 * Supports:
 * - event: citation -> Citation object
 * - event: message -> { delta: string }
 * - event: done -> { total_tokens: number, duration_ms: number }
 * - event: error -> { message: string }
 */
export async function streamChat(
  projectId: string,
  req: ChatCompletionRequest,
  callbacks: StreamChatCallbacks,
  signal?: AbortSignal
): Promise<void> {
  const url = `${API_BASE_URL}/projects/${projectId}/chat/completions/stream`;

  if (signal?.aborted) return;

  let response: Response;
  try {
    const streamHeaders: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    };
    if (typeof window !== "undefined") {
      const token = localStorage.getItem("cf_token");
      if (token) {
        streamHeaders["Authorization"] = `Bearer ${token}`;
      }
    }

    response = await fetch(url, {
      method: "POST",
      headers: streamHeaders,
      credentials: "include",
      body: JSON.stringify(req),
      signal,
    });
  } catch (err: any) {
    if (signal?.aborted || err?.name === "AbortError") {
      return;
    }
    callbacks.onError(err);
    return;
  }

  if (!response.ok) {
    try {
      const errJson = await response.json();
      callbacks.onError(
        new ApiError(
          response.status,
          errJson.detail || errJson.title || "Failed to stream chat",
          errJson
        )
      );
    } catch {
      callbacks.onError(
        new ApiError(
          response.status,
          `Chat stream request failed: ${response.statusText}`
        )
      );
    }
    return;
  }

  if (!response.body) {
    callbacks.onError(new Error("Response body is empty"));
    return;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  try {
    while (true) {
      if (signal?.aborted) {
        try {
          await reader.cancel();
        } catch {
          // ignore cancel error
        }
        break;
      }

      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });

      // SSE frames are separated by double newlines (\n\n or \r\n\r\n)
      const frames = buffer.split(/\r?\n\r?\n/);
      // Keep incomplete trailing frame in buffer
      buffer = frames.pop() ?? "";

      for (const frame of frames) {
        if (!frame.trim()) continue;

        let currentEvent = "message";
        const dataLines: string[] = [];

        const lines = frame.split(/\r?\n/);
        for (const line of lines) {
          if (line.startsWith("event:")) {
            currentEvent = line.substring(6).trim();
          } else if (line.startsWith("data:")) {
            dataLines.push(line.substring(5).trim());
          }
        }

        const dataStr = dataLines.join("\n");
        if (!dataStr) continue;

        if (dataStr === "[DONE]") {
          callbacks.onDone({ total_tokens: 0, duration_ms: 0 });
          return;
        }

        try {
          const parsed = JSON.parse(dataStr);

          if (currentEvent === "citation") {
            callbacks.onCitation(parsed as Citation);
          } else if (currentEvent === "message" || currentEvent === "token") {
            const token = parsed.delta ?? parsed.token ?? parsed.text ?? "";
            callbacks.onToken(token);
          } else if (currentEvent === "done") {
            callbacks.onDone(parsed);
          } else if (currentEvent === "error") {
            const errText = parsed.error || parsed.message || parsed.detail || "Streaming error";
            callbacks.onError(new Error(errText));
          } else {
            // Fallback for untyped events
            if (parsed.delta !== undefined) {
              callbacks.onToken(parsed.delta);
            } else if (parsed.file_path && parsed.snippet !== undefined) {
              callbacks.onCitation(parsed as Citation);
            }
          }
        } catch {
          // If not valid JSON, treat plain text data as token
          if (currentEvent === "message" || currentEvent === "token") {
            callbacks.onToken(dataStr);
          }
        }
      }
    }

    // Process any remainder in buffer
    if (buffer.trim()) {
      try {
        const lines = buffer.split(/\r?\n/);
        let event = "message";
        const dataLines: string[] = [];
        for (const line of lines) {
          if (line.startsWith("event:")) event = line.substring(6).trim();
          else if (line.startsWith("data:")) dataLines.push(line.substring(5).trim());
        }
        const dataStr = dataLines.join("\n");
        if (dataStr && dataStr !== "[DONE]") {
          const parsed = JSON.parse(dataStr);
          if (event === "citation") callbacks.onCitation(parsed);
          else if (event === "done") callbacks.onDone(parsed);
          else if (event === "error") callbacks.onError(new Error(parsed.error || parsed.message || parsed.detail || "Streaming error"));
          else callbacks.onToken(parsed.delta ?? dataStr);
        }
      } catch {
        // Ignore parsing errors on stream end
      }
    }
  } catch (streamErr: any) {
    if (signal?.aborted || streamErr?.name === "AbortError") {
      return;
    }
    callbacks.onError(streamErr);
  } finally {
    try {
      reader.releaseLock();
    } catch {
      // ignore
    }
  }
}

// ---------------------------------------------------------------------------
// Database Sources API
// ---------------------------------------------------------------------------

export async function testDatabaseConnection(
  projectId: string,
  req: { database_type: string; connection_url: string }
): Promise<ConnectionTestResult> {
  return apiFetch<ConnectionTestResult>(`/projects/${projectId}/sources/database/test`, {
    method: "POST",
    body: JSON.stringify(req),
  });
}

export async function createDatabaseSource(
  projectId: string,
  req: CreateDatabaseSourceRequest
): Promise<DatabaseSource> {
  return apiFetch<DatabaseSource>(`/projects/${projectId}/sources/database`, {
    method: "POST",
    body: JSON.stringify(req),
  });
}

export async function getDatabaseSources(projectId: string): Promise<DatabaseSource[]> {
  return apiFetch<DatabaseSource[]>(`/projects/${projectId}/sources/database`);
}

export async function getDatabaseSource(
  projectId: string,
  sourceId: string
): Promise<DatabaseSource> {
  return apiFetch<DatabaseSource>(`/projects/${projectId}/sources/database/${sourceId}`);
}

export async function updateDatabaseSource(
  projectId: string,
  sourceId: string,
  req: UpdateDatabaseSourceRequest
): Promise<DatabaseSource> {
  return apiFetch<DatabaseSource>(`/projects/${projectId}/sources/database/${sourceId}`, {
    method: "PATCH",
    body: JSON.stringify(req),
  });
}

export async function deleteDatabaseSource(
  projectId: string,
  sourceId: string
): Promise<void> {
  return apiFetch<void>(`/projects/${projectId}/sources/database/${sourceId}`, {
    method: "DELETE",
  });
}

export async function testStoredDatabaseConnection(
  projectId: string,
  sourceId: string
): Promise<ConnectionTestResult> {
  return apiFetch<ConnectionTestResult>(
    `/projects/${projectId}/sources/database/${sourceId}/test`,
    {
      method: "POST",
    }
  );
}

export async function getDatabaseMetadata(
  projectId: string,
  sourceId: string
): Promise<DatabaseMetadata> {
  return apiFetch<DatabaseMetadata>(
    `/projects/${projectId}/sources/database/${sourceId}/metadata`
  );
}

export async function syncDatabaseSource(
  projectId: string,
  sourceId: string
): Promise<{ job_id: string; status: string }> {
  return apiFetch<{ job_id: string; status: string }>(
    `/projects/${projectId}/sources/database/${sourceId}/sync`,
    {
      method: "POST",
    }
  );
}

export async function getDatabaseSourceStatus(
  projectId: string,
  sourceId: string
): Promise<{ source_id: string; status: string; last_error?: string; last_synced_at?: string }> {
  return apiFetch<{
    source_id: string;
    status: string;
    last_error?: string;
    last_synced_at?: string;
  }>(`/projects/${projectId}/sources/database/${sourceId}/status`);
}

