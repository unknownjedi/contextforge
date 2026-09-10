/**
 * ContextForge TypeScript API Definitions
 * Mirrored from docs/api/openapi.yaml
 */

export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
}

export interface User {
  id: string; // UUID
  github_login: string;
  email: string;
  name?: string;
  avatar_url?: string;
  created_at?: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface Project {
  id: string; // UUID
  name: string;
  description?: string;
  owner_user_id: string; // UUID
  embedding_provider?: string;
  embedding_model?: string;
  embedding_dimension: number;
  llm_provider?: string;
  total_documents?: number;
  total_chunks?: number;
  total_sources?: number;
  status?: "ready" | "syncing" | "failed" | "idle";
  created_at: string;
  updated_at: string;
}

export interface CreateProjectRequest {
  name: string;
  description?: string;
  embedding_provider?: string;
  embedding_model?: string;
  embedding_dimension?: number;
  llm_provider?: string;
}

export interface UpdateProjectRequest {
  name?: string;
  description?: string;
  llm_provider?: string;
}

export interface ProjectListResponse {
  items: Project[];
  total: number;
  page?: number;
  page_size?: number;
}

export type SourceSyncStatus = "idle" | "queued" | "syncing" | "synced" | "failed";

export interface Source {
  id: string; // UUID
  project_id: string; // UUID
  name: string;
  repo_owner: string;
  repo_name: string;
  branch: string;
  last_commit_hash?: string;
  sync_status: SourceSyncStatus;
  last_synced_at?: string;
  created_at?: string;
}

export type AuthMethod = "public" | "oauth" | "pat";

export interface CreateSourceRequest {
  name?: string;
  type?: string;
  repo_url?: string;
  repo_owner?: string;
  repo_name?: string;
  branch?: string;
  auth_method?: AuthMethod;
  pat_token?: string;
  url?: string;
}

export interface SourceIngestionQueued {
  source: Source;
  job_id: string; // UUID
}

export type JobStatus = "pending" | "running" | "completed" | "failed";

export interface IngestionJob {
  id: string; // UUID
  project_id: string; // UUID
  source_id: string; // UUID
  status: JobStatus;
  progress_percent: number;
  processed_files?: number;
  total_files?: number;
  error_message?: string;
  created_at?: string;
  updated_at?: string;
  finished_at?: string;
}

export interface Document {
  id: string; // UUID
  project_id: string; // UUID
  source_id: string; // UUID
  file_path: string;
  language?: string;
  content_hash: string;
  total_chunks?: number;
  created_at?: string;
  updated_at?: string;
}

export interface DocumentChunk {
  id: string; // UUID
  chunk_index: number;
  start_line: number;
  end_line: number;
  content: string;
  token_count?: number;
}

export interface DocumentDetail extends Document {
  chunks?: DocumentChunk[];
}

export interface DocumentListResponse {
  items: Document[];
  total: number;
}

export interface UploadDocumentResponse {
  id: string;
  project_id: string;
  source_id: string;
  file_path: string;
  language: string;
  content_hash: string;
  total_chunks: number;
  created_at: string;
  document?: Document;
  chunk_count?: number;
}

export interface ChatCompletionRequest {
  message: string;
  provider?: string;
  model?: string;
  top_k?: number;
  similarity_threshold?: number;
  file_filters?: string[];
  temperature?: number;
  conversation_id?: string;
}

export interface Citation {
  source_id: string; // UUID
  file_path: string;
  start_line: number;
  end_line: number;
  similarity: number;
  snippet: string;
}

export interface ChatCompletionResponse {
  answer: string;
  citations: Citation[];
  tokens_used: number;
  duration_ms: number;
}

export interface ChatMessageRecord {
  id: string;
  conversation_id: string;
  role: "user" | "assistant" | "system";
  content: string;
  citations?: Citation[];
  tokens_used?: number;
  duration_ms?: number;
  created_at: string;
}

export interface Conversation {
  id: string;
  project_id: string;
  title: string;
  created_at: string;
  updated_at: string;
  messages?: ChatMessageRecord[];
}

export interface CreateConversationRequest {
  title?: string;
}

export type StreamEvent =
  | { event: "citation"; data: Citation }
  | { event: "message"; data: { delta: string } }
  | { event: "done"; data: { total_tokens: number; duration_ms: number } }
  | { event: "error"; data: { message: string } };

export type DatabaseType =
  | "postgres"
  | "mysql"
  | "mariadb"
  | "sqlite"
  | "sqlserver"
  | "cockroachdb";

export interface DatabaseSource {
  id: string; // UUID
  source_id: string; // UUID
  project_id: string; // UUID
  name: string;
  database_type: DatabaseType;
  host: string;
  port: number;
  database_name: string;
  username: string;
  configuration: Record<string, any>;
  status: "created" | "configured" | "syncing" | "ready" | "failed";
  last_error?: string;
  last_synced_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateDatabaseSourceRequest {
  name: string;
  database_type: DatabaseType;
  connection_url: string;
  configuration?: {
    mode?: "schema" | "schema_and_data";
    schemas?: string[];
    tables?: string[];
    excluded_columns?: Record<string, string[]>;
  };
}

export interface UpdateDatabaseSourceRequest {
  name?: string;
  configuration?: Record<string, any>;
}

export interface ConnectionTestResult {
  success: boolean;
  database_type: string;
  database_version?: string;
  latency_ms?: number;
  error_message?: string;
}

export interface ColumnMetadata {
  name: string;
  data_type: string;
  nullable: boolean;
  default_value?: string;
  comment?: string;
  position: number;
  is_primary_key: boolean;
  is_sensitive: boolean;
}

export interface ForeignKeyMetadata {
  name?: string;
  columns: string[];
  referenced_schema: string;
  referenced_table: string;
  referenced_columns: string[];
}

export interface IndexMetadata {
  name: string;
  columns: string[];
  unique: boolean;
}

export interface TableMetadata {
  schema: string;
  name: string;
  type: "table" | "view";
  comment?: string;
  columns: ColumnMetadata[];
  primary_key?: string[];
  foreign_keys?: ForeignKeyMetadata[];
  indexes?: IndexMetadata[];
}

export interface SchemaMetadata {
  name: string;
  tables: TableMetadata[];
}

export interface DatabaseMetadata {
  database_type: string;
  database_name: string;
  version?: string;
  schemas: SchemaMetadata[];
}

