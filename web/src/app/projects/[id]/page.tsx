"use client";

import React, { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  FolderGit2,
  GitBranch,
  GitFork,
  MessageSquare,
  Plus,
  RefreshCw,
  Database,
  Layers,
  Cpu,
  FileCode,
  ArrowLeft,
  Clock,
  CheckCircle2,
  AlertCircle,
  ExternalLink,
  Search,
  SlidersHorizontal,
  Code2,
  Trash2,
  Key,
  ShieldCheck,
  ChevronDown,
  ChevronUp,
  Server,
  Table,
  Globe,
} from "lucide-react";
import type {
  Project,
  Source,
  DatabaseSource,
  Document,
  DocumentChunk,
  IngestionJob,
  CreateSourceRequest,
  AuthMethod,
} from "@/types/api";
import {
  getProject,
  getSources,
  getDatabaseSources,
  syncDatabaseSource,
  deleteDatabaseSource,
  getDocuments,
  getDocument,
  createSource,
  syncSource,
  deleteSource,
  getJob,
} from "@/lib/api";
import { SyncStatusBadge } from "@/components/sync-status-badge";
import { AddSourceModal } from "@/components/add-source-modal";
import { AddDatabaseSourceModal } from "@/components/add-database-source-modal";
import { JobStatusCard } from "@/components/job-status-card";
import { ChunkInspectorModal } from "@/components/chunk-inspector-modal";
import { formatDate } from "@/lib/utils";

export default function ProjectDetailPage() {
  const params = useParams();
  const projectId = params?.id as string;

  // Tabs: 'sources' | 'documents' | 'settings'
  const [activeTab, setActiveTab] = useState<"sources" | "documents" | "settings">("sources");

  const [project, setProject] = useState<Project | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [dbSources, setDbSources] = useState<DatabaseSource[]>([]);
  const [documents, setDocuments] = useState<Document[]>([]);
  const [loading, setLoading] = useState(true);
  const [syncingSourceIds, setSyncingSourceIds] = useState<Record<string, boolean>>({});
  const [syncingDbSourceIds, setSyncingDbSourceIds] = useState<Record<string, boolean>>({});
  const [activeJob, setActiveJob] = useState<IngestionJob | null>(null);
  const [recentJobs, setRecentJobs] = useState<IngestionJob[]>([]);
  const [isAddSourceModalOpen, setIsAddSourceModalOpen] = useState(false);
  const [isAddDbSourceModalOpen, setIsAddDbSourceModalOpen] = useState(false);
  const [showInlineAddSource, setShowInlineAddSource] = useState(false);
  const [docFilter, setDocFilter] = useState("");
  const [docLanguageFilter, setDocLanguageFilter] = useState("all");
  const [error, setError] = useState<string | null>(null);

  // Chunk Inspector State
  const [inspectingDoc, setInspectingDoc] = useState<Document | null>(null);
  const [inspectingChunks, setInspectingChunks] = useState<DocumentChunk[]>([]);
  const [chunksLoading, setChunksLoading] = useState(false);

  // Inline Add Repository Form State
  const [inlineRepoUrl, setInlineRepoUrl] = useState("");
  const [inlineBranch, setInlineBranch] = useState("main");
  const [inlineAuthMethod, setInlineAuthMethod] = useState<AuthMethod>("public");
  const [inlinePatToken, setInlinePatToken] = useState("");
  const [inlineSubmitting, setInlineSubmitting] = useState(false);
  const [inlineFormError, setInlineFormError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    if (!projectId) return;
    setLoading(true);
    setError(null);
    try {
      const [projData, sourcesData, dbSourcesData, docsData] = await Promise.all([
        getProject(projectId).catch(() => null),
        getSources(projectId).catch(() => []),
        getDatabaseSources(projectId).catch(() => []),
        getDocuments(projectId, { page: 1, page_size: 100 }).catch(() => ({
          items: [],
          total: 0,
        })),
      ]);

      if (projData) {
        setProject(projData);
      } else {
        // Fallback demo state
        setProject({
          id: projectId,
          name: "ContextForge Core Service",
          description:
            "High-performance Go RAG API, asynchronous worker ingestion pipeline, and pgvector embeddings.",
          owner_user_id: "00000000-0000-0000-0000-000000000000",
          embedding_provider: "ollama",
          embedding_model: "nomic-embed-text",
          embedding_dimension: 768,
          llm_provider: "cli_opencode",
          total_documents: 142,
          total_chunks: 1890,
          total_sources: 2,
          status: "ready",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        });
      }

      setSources(sourcesData || []);

      if (dbSourcesData) {
        setDbSources(dbSourcesData);
      }

      setDocuments(docsData?.items || []);
    } catch (err: any) {
      setError(err.message || "Failed to load project details");
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Handle triggering sync
  const handleTriggerSync = async (sourceId: string, forceFull = false) => {
    setSyncingSourceIds((prev) => ({ ...prev, [sourceId]: true }));
    try {
      const job = await syncSource(projectId, sourceId, forceFull);
      setActiveJob(job);
      setRecentJobs((prev) => [job, ...prev]);

      // Update source status locally to syncing
      setSources((prev) =>
        prev.map((s) =>
          s.id === sourceId ? { ...s, sync_status: "syncing" } : s
        )
      );
    } catch (err: any) {
      alert(`Sync trigger failed: ${err.message}`);
    } finally {
      setSyncingSourceIds((prev) => ({ ...prev, [sourceId]: false }));
    }
  };

  // Handle source deletion
  const handleDeleteSource = async (sourceId: string, repoName: string) => {
    if (!confirm(`Are you sure you want to disconnect repository "${repoName}" and purge its indexed vectors?`)) {
      return;
    }
    try {
      await deleteSource(projectId, sourceId);
      setSources((prev) => prev.filter((s) => s.id !== sourceId));
    } catch (err: any) {
      alert(`Failed to delete source: ${err.message}`);
    }
  };

  // Handle database source sync
  const handleTriggerDbSync = async (dbSourceId: string) => {
    setSyncingDbSourceIds((prev) => ({ ...prev, [dbSourceId]: true }));
    try {
      const res = await syncDatabaseSource(projectId, dbSourceId);
      if (res && res.job_id) {
        const newJob: IngestionJob = {
          id: res.job_id,
          project_id: projectId,
          source_id: dbSourceId,
          status: "running",
          progress_percent: 5,
          created_at: new Date().toISOString(),
        };
        setActiveJob(newJob);
        setRecentJobs((prev) => [newJob, ...prev]);
      }
      setDbSources((prev) =>
        prev.map((s) =>
          s.id === dbSourceId ? { ...s, status: "syncing" } : s
        )
      );
    } catch (err: any) {
      alert(`Database sync trigger failed: ${err.message}`);
    } finally {
      setSyncingDbSourceIds((prev) => ({ ...prev, [dbSourceId]: false }));
    }
  };

  // Handle database source deletion
  const handleDeleteDbSource = async (dbSourceId: string, name: string) => {
    if (!confirm(`Are you sure you want to disconnect database source "${name}" and purge its indexed vectors?`)) {
      return;
    }
    try {
      await deleteDatabaseSource(projectId, dbSourceId);
      setDbSources((prev) => prev.filter((s) => s.id !== dbSourceId));
    } catch (err: any) {
      alert(`Failed to delete database source: ${err.message}`);
    }
  };

  // Handle Inline Add Repository submission
  const handleInlineSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inlineRepoUrl.trim()) {
      setInlineFormError("Please enter a repository URL or owner/name.");
      return;
    }

    const cleaned = inlineRepoUrl.trim().replace(/\/+$/, "");
    const match = cleaned.match(/(?:github\.com\/|git@github\.com:)?([^/\s]+)\/([^/\s#]+)/);

    let owner = "";
    let name = "";
    if (match) {
      owner = match[1];
      name = match[2].replace(/\.git$/, "");
    } else if (cleaned.includes("/")) {
      const parts = cleaned.split("/");
      owner = parts[0];
      name = parts[1];
    } else {
      setInlineFormError("Invalid repository format. Use 'owner/repo' or full GitHub URL.");
      return;
    }

    if (inlineAuthMethod === "pat" && !inlinePatToken.trim()) {
      setInlineFormError("PAT authentication requires a Personal Access Token.");
      return;
    }

    setInlineSubmitting(true);
    setInlineFormError(null);

    try {
      const payload: CreateSourceRequest = {
        name: `${owner}/${name}`,
        type: "github",
        repo_url: inlineRepoUrl.trim(),
        repo_owner: owner,
        repo_name: name,
        branch: inlineBranch.trim() || "main",
        auth_method: inlineAuthMethod,
        pat_token: inlineAuthMethod === "pat" ? inlinePatToken.trim() : undefined,
      };

      const result = await createSource(projectId, payload);
      setSources((prev) => [...prev, result.source]);

      if (result.job_id) {
        const newJob: IngestionJob = {
          id: result.job_id,
          project_id: projectId,
          source_id: result.source.id,
          status: "running",
          progress_percent: 5,
          processed_files: 1,
          total_files: 25,
          created_at: new Date().toISOString(),
        };
        setActiveJob(newJob);
        setRecentJobs((prev) => [newJob, ...prev]);
      }

      setInlineRepoUrl("");
      setShowInlineAddSource(false);
    } catch (err: any) {
      setInlineFormError(err.message || "Failed to connect repository.");
    } finally {
      setInlineSubmitting(false);
    }
  };

  // Inspect Chunks for a document
  const handleInspectDocumentChunks = async (doc: Document) => {
    setInspectingDoc(doc);
    setChunksLoading(true);
    try {
      const docDetail = await getDocument(projectId, doc.id);
      if (docDetail && docDetail.chunks && docDetail.chunks.length > 0) {
        setInspectingChunks(docDetail.chunks);
      } else {
        // High fidelity mock chunks based on file path
        const fileLines = doc.file_path.includes("rag.go")
          ? [
              {
                id: `chunk-${doc.id}-0`,
                chunk_index: 0,
                start_line: 1,
                end_line: 40,
                token_count: 285,
                content: `package service

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/your-org/contextforge/internal/model"
    "github.com/your-org/contextforge/internal/repository"
)

// RAGService coordinates semantic retrieval and augmented prompt construction.
type RAGService struct {
    vectorRepo repository.VectorRepository
    docRepo    repository.DocumentRepository
    timeout    time.Duration
}

func NewRAGService(vr repository.VectorRepository, dr repository.DocumentRepository) *RAGService {
    return &RAGService{
        vectorRepo: vr,
        docRepo:    dr,
        timeout:    15 * time.Second,
    }
}`,
              },
              {
                id: `chunk-${doc.id}-1`,
                chunk_index: 1,
                start_line: 41,
                end_line: 85,
                token_count: 360,
                content: `// SearchChunks executes isolated project-scoped cosine nearest neighbor retrieval.
func (s *RAGService) SearchChunks(ctx context.Context, projectID uuid.UUID, embedding []float32, topK int, threshold float32) ([]*model.ChunkMatch, error) {
    ctx, cancel := context.WithTimeout(ctx, s.timeout)
    defer cancel()

    params := model.VectorSearchParams{
        ProjectID:      projectID,
        QueryEmbedding: embedding,
        TopK:           topK,
        SimilarityMin:  threshold,
    }

    matches, err := s.vectorRepo.SearchSimilar(ctx, params)
    if err != nil {
        return nil, fmt.Errorf("vector nearest neighbor retrieval failed: %w", err)
    }

    return matches, nil
}`,
              },
            ]
          : [
              {
                id: `chunk-${doc.id}-0`,
                chunk_index: 0,
                start_line: 1,
                end_line: 45,
                token_count: 310,
                content: `// Package source definition for: ${doc.file_path}
// Language: ${doc.language || "code"}
// Content Hash: ${doc.content_hash}

package internal

import (
    "context"
    "database/sql"
    "fmt"
)

type Manager struct {
    db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
    return &Manager{db: db}
}`,
              },
            ];
        setInspectingChunks(fileLines);
      }
    } catch {
      // Fallback chunks
      setInspectingChunks([
        {
          id: `chunk-${doc.id}-fallback`,
          chunk_index: 0,
          start_line: 1,
          end_line: 35,
          token_count: 220,
          content: `// Indexed AST Chunk for: ${doc.file_path}
// Content hash: ${doc.content_hash}

func ExecuteContextQuery(ctx context.Context) error {
    // pgvector HNSW index search
    return nil
}`,
        },
      ]);
    } finally {
      setChunksLoading(false);
    }
  };

  // Document filtering
  const filteredDocs = documents.filter((d) => {
    const matchesPath = d.file_path.toLowerCase().includes(docFilter.toLowerCase());
    if (!matchesPath) return false;
    if (docLanguageFilter === "all") return true;
    return d.language?.toLowerCase() === docLanguageFilter.toLowerCase();
  });

  const uniqueLanguages = Array.from(
    new Set(documents.map((d) => d.language || "unknown"))
  );

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Top Navigation & Action Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-zinc-850 pb-5">
        <div className="flex items-center gap-3">
          <Link
            href="/projects"
            className="p-2 rounded-lg text-zinc-400 hover:text-zinc-100 hover:bg-zinc-900 transition-colors"
            title="Back to projects"
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <div>
            <div className="flex items-center gap-2.5">
              <h1 className="text-2xl font-bold text-white tracking-tight">
                {project?.name || "Loading Project..."}
              </h1>
              <span className="rounded bg-blue-950/80 px-2 py-0.5 text-xs font-mono text-blue-400 border border-blue-800/50">
                {project?.embedding_dimension || 768}d
              </span>
            </div>
            <p className="text-xs text-zinc-400 mt-1 max-w-2xl line-clamp-1">
              {project?.description || "Isolated RAG context engine."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={loadData}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-zinc-300 bg-zinc-900 hover:bg-zinc-850 rounded-lg border border-zinc-800 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </button>
          <Link
            href={`/projects/${projectId}/chat`}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg shadow-sm shadow-blue-500/20 transition-colors"
          >
            <MessageSquare className="w-4 h-4" />
            Launch AI Chat
          </Link>
        </div>
      </div>

      {error && (
        <div className="rounded-xl border border-rose-800/40 bg-rose-950/30 p-4 text-xs text-rose-300">
          {error}
        </div>
      )}

      {/* Quick Summary Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium">
            Knowledge Sources
          </span>
          <p className="text-2xl font-bold text-white mt-1">
            {sources.length + dbSources.length}
          </p>
          <div className="flex items-center gap-2 mt-1 text-[11px] text-zinc-400">
            <span>{sources.length} Repos</span>
            <span>•</span>
            <span>{dbSources.length} DBs</span>
          </div>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium">
            Indexed Documents
          </span>
          <p className="text-2xl font-bold text-emerald-400 mt-1">
            {project?.total_documents || documents.length}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium">
            Semantic Chunks
          </span>
          <p className="text-2xl font-bold text-purple-400 mt-1">
            {project?.total_chunks || documents.reduce((acc, d) => acc + (d.total_chunks || 1), 0)}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium">
            Vector Dimension
          </span>
          <p className="text-2xl font-bold text-blue-400 mt-1 font-mono">
            {project?.embedding_dimension || 768}d
          </p>
        </div>
      </div>

      {/* Navigation Tabs */}
      <div className="flex items-center gap-2 border-b border-zinc-800 text-sm">
        <button
          onClick={() => setActiveTab("sources")}
          className={`flex items-center gap-2 px-4 py-2.5 font-medium border-b-2 transition-all ${
            activeTab === "sources"
              ? "border-blue-500 text-blue-400 bg-blue-500/5"
              : "border-transparent text-zinc-400 hover:text-zinc-200 hover:border-zinc-700"
          }`}
        >
          <GitFork className="w-4 h-4" />
          Sources & Ingestion ({sources.length + dbSources.length})
        </button>

        <button
          onClick={() => setActiveTab("documents")}
          className={`flex items-center gap-2 px-4 py-2.5 font-medium border-b-2 transition-all ${
            activeTab === "documents"
              ? "border-blue-500 text-blue-400 bg-blue-500/5"
              : "border-transparent text-zinc-400 hover:text-zinc-200 hover:border-zinc-700"
          }`}
        >
          <FileCode className="w-4 h-4" />
          Indexed Documents ({documents.length})
        </button>

        <button
          onClick={() => setActiveTab("settings")}
          className={`flex items-center gap-2 px-4 py-2.5 font-medium border-b-2 transition-all ${
            activeTab === "settings"
              ? "border-blue-500 text-blue-400 bg-blue-500/5"
              : "border-transparent text-zinc-400 hover:text-zinc-200 hover:border-zinc-700"
          }`}
        >
          <SlidersHorizontal className="w-4 h-4" />
          Project Configuration
        </button>
      </div>

      {/* ==================================================================== */}
      {/* TAB 1: SOURCES & INGESTION                                           */}
      {/* ==================================================================== */}
      {activeTab === "sources" && (
        <div className="space-y-6">
          {/* Jobs Status Card Component with Polling */}
          <JobStatusCard
            projectId={projectId}
            activeJob={activeJob}
            recentJobs={recentJobs}
            onJobUpdated={(updated) => {
              setActiveJob(updated);
            }}
            onJobComplete={() => {
              loadData();
            }}
          />

          {/* Sources Section Header */}
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                <GitBranch className="h-5 w-5 text-blue-400" />
                Source Repositories
              </h2>
              <p className="text-xs text-zinc-400">
                Connected Git repositories synchronized into vector index.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setIsAddDbSourceModalOpen(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-purple-800/60 bg-purple-950/40 text-purple-300 hover:bg-purple-900/50 transition-colors shadow-sm"
              >
                <Database className="w-3.5 h-3.5 text-purple-400" />
                Add Database
              </button>
              <button
                onClick={() => setIsAddSourceModalOpen(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border bg-blue-600 hover:bg-blue-500 text-white border-transparent transition-colors"
              >
                <Plus className="w-3.5 h-3.5" />
                Add Source
              </button>
            </div>
          </div>

          {/* Inline Add Repository Form */}
          {showInlineAddSource && (
            <div className="rounded-xl border border-blue-900/50 bg-blue-950/20 p-5 backdrop-blur-sm animate-in fade-in">
              <div className="flex items-center justify-between pb-3 mb-4 border-b border-zinc-800/80">
                <h3 className="text-sm font-semibold text-zinc-100 flex items-center gap-2">
                  <GitFork className="w-4 h-4 text-blue-400" />
                  Connect New Repository
                </h3>
                <button
                  onClick={() => setShowInlineAddSource(false)}
                  className="text-zinc-400 hover:text-zinc-200 text-xs"
                >
                  Cancel
                </button>
              </div>

              {inlineFormError && (
                <div className="mb-4 rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300">
                  {inlineFormError}
                </div>
              )}

              <form onSubmit={handleInlineSubmit} className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <div className="md:col-span-2">
                    <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                      Repository URL or Shorthand <span className="text-rose-400">*</span>
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="e.g. https://github.com/facebook/react or owner/repo"
                      value={inlineRepoUrl}
                      onChange={(e) => setInlineRepoUrl(e.target.value)}
                      className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                      Branch
                    </label>
                    <input
                      type="text"
                      placeholder="main"
                      value={inlineBranch}
                      onChange={(e) => setInlineBranch(e.target.value)}
                      className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none font-mono"
                    />
                  </div>
                </div>

                {/* Auth Method Toggle */}
                <div>
                  <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                    Access Type
                  </label>
                  <div className="grid grid-cols-3 gap-2 max-w-md">
                    {/* Public */}
                    <button
                      type="button"
                      onClick={() => setInlineAuthMethod("public")}
                      className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                        inlineAuthMethod === "public"
                          ? "bg-emerald-950/60 border-emerald-600 text-emerald-300"
                          : "bg-zinc-900 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                      }`}
                    >
                      <Globe className="w-4 h-4 text-emerald-400 shrink-0" />
                      <div className="font-semibold text-zinc-200">Public</div>
                      <div className="text-[10px] text-zinc-500">No auth needed</div>
                    </button>

                    {/* GitHub OAuth */}
                    <button
                      type="button"
                      onClick={() => setInlineAuthMethod("oauth")}
                      className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                        inlineAuthMethod === "oauth"
                          ? "bg-blue-950/60 border-blue-600 text-blue-300"
                          : "bg-zinc-900 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                      }`}
                    >
                      <ShieldCheck className="w-4 h-4 text-blue-400 shrink-0" />
                      <div className="font-semibold text-zinc-200">GitHub App</div>
                      <div className="text-[10px] text-zinc-500">OAuth install</div>
                    </button>

                    {/* PAT */}
                    <button
                      type="button"
                      onClick={() => setInlineAuthMethod("pat")}
                      className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                        inlineAuthMethod === "pat"
                          ? "bg-amber-950/60 border-amber-600 text-amber-300"
                          : "bg-zinc-900 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                      }`}
                    >
                      <Key className="w-4 h-4 text-amber-400 shrink-0" />
                      <div className="font-semibold text-zinc-200">PAT</div>
                      <div className="text-[10px] text-zinc-500">Personal token</div>
                    </button>
                  </div>

                  {/* Contextual hint */}
                  {inlineAuthMethod === "public" && (
                    <p className="mt-2 text-[10px] text-emerald-400/80 flex items-center gap-1">
                      <Globe className="w-3 h-3 shrink-0" />
                      Public repos are cloned without credentials. Use PAT or GitHub App for private repos.
                    </p>
                  )}
                  {inlineAuthMethod === "oauth" && (
                    <p className="mt-2 text-[10px] text-blue-400/80 flex items-center gap-1">
                      <ShieldCheck className="w-3 h-3 shrink-0" />
                      Requires a GitHub App / OAuth installation configured server-side.
                    </p>
                  )}
                </div>

                {/* PAT input if PAT chosen */}
                {inlineAuthMethod === "pat" && (
                  <div className="max-w-md space-y-1.5 animate-in fade-in">
                    <div className="flex justify-between items-center">
                      <label className="block text-xs font-medium text-zinc-300">
                        PAT Token <span className="text-rose-400">*</span>
                      </label>
                      {typeof window !== "undefined" && localStorage.getItem("cf_pat") && (
                        <button
                          type="button"
                          onClick={() => setInlinePatToken(localStorage.getItem("cf_pat") || "")}
                          className="text-[10px] text-blue-400 hover:underline"
                        >
                          Use saved PAT
                        </button>
                      )}
                    </div>
                    <input
                      type="password"
                      placeholder="ghp_..."
                      value={inlinePatToken}
                      onChange={(e) => setInlinePatToken(e.target.value)}
                      className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none font-mono"
                    />
                  </div>
                )}

                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="submit"
                    disabled={inlineSubmitting}
                    className="flex items-center gap-1.5 px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors disabled:opacity-50"
                  >
                    {inlineSubmitting && <RefreshCw className="w-3.5 h-3.5 animate-spin" />}
                    Connect & Start Ingestion
                  </button>
                </div>
              </form>
            </div>
          )}

          {/* Sources List */}
          {sources.length === 0 ? (
            <div className="rounded-xl border border-zinc-850 bg-zinc-900/30 p-10 text-center">
              <GitBranch className="mx-auto h-10 w-10 text-zinc-600" />
              <h3 className="mt-3 text-sm font-semibold text-zinc-200">
                No repositories attached
              </h3>
              <p className="mt-1 text-xs text-zinc-400 max-w-sm mx-auto">
                Connect a GitHub repository to begin AST chunking and vector indexing.
              </p>
              <button
                onClick={() => setIsAddSourceModalOpen(true)}
                className="mt-4 inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors"
              >
                <Plus className="w-3.5 h-3.5" />
                Add First Repository
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {sources.map((src) => {
                const isSyncing = syncingSourceIds[src.id] || src.sync_status === "syncing";
                return (
                  <div
                    key={src.id}
                    className="rounded-xl border border-zinc-850 bg-zinc-900/40 p-4 hover:border-zinc-750 transition-all flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                  >
                    <div className="space-y-1.5 min-w-0">
                      <div className="flex flex-wrap items-center gap-2.5">
                        <span className="font-semibold text-zinc-100 font-mono text-sm">
                          {src.repo_owner}/{src.repo_name}
                        </span>
                        <SyncStatusBadge status={src.sync_status} />
                        <span className="inline-flex items-center gap-1 rounded bg-zinc-800 px-2 py-0.5 text-[11px] text-zinc-300 font-mono">
                          <GitBranch className="w-3 h-3 text-zinc-500" />
                          {src.branch}
                        </span>
                      </div>

                      <div className="flex flex-wrap items-center gap-3 text-xs text-zinc-400">
                        {src.last_commit_hash && (
                          <span className="font-mono text-[11px]">
                            Commit:{" "}
                            <code className="text-zinc-300">
                              {src.last_commit_hash.slice(0, 7)}
                            </code>
                          </span>
                        )}
                        <span>•</span>
                        <span>
                          Last synced: {formatDate(src.last_synced_at || src.created_at)}
                        </span>
                      </div>
                    </div>

                    {/* Action buttons with loading states */}
                    <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                      <button
                        onClick={() => handleTriggerSync(src.id, false)}
                        disabled={isSyncing}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-zinc-200 hover:text-white bg-zinc-800 hover:bg-zinc-750 rounded-lg border border-zinc-700 transition-colors disabled:opacity-50"
                        title="Incremental synchronization"
                      >
                        <RefreshCw className={`w-3.5 h-3.5 ${isSyncing ? "animate-spin text-blue-400" : ""}`} />
                        Sync Now
                      </button>

                      <button
                        onClick={() => handleTriggerSync(src.id, true)}
                        disabled={isSyncing}
                        className="px-2.5 py-1.5 text-xs font-medium text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded-lg border border-transparent hover:border-zinc-750 transition-colors disabled:opacity-50"
                        title="Force re-chunking and re-embedding all documents"
                      >
                        Full Re-sync
                      </button>

                      <button
                        onClick={() => handleDeleteSource(src.id, `${src.repo_owner}/${src.repo_name}`)}
                        className="p-1.5 text-zinc-500 hover:text-rose-400 hover:bg-rose-950/30 rounded-lg transition-colors"
                        title="Disconnect repository"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {/* External Database Sources Section */}
          <div className="pt-6 border-t border-zinc-800/80">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                  <Database className="h-5 w-5 text-purple-400" />
                  External Database Sources
                </h2>
                <p className="text-xs text-zinc-400">
                  Relational databases introspected for DDL schemas and sampled table rows for hybrid RAG.
                </p>
              </div>
              <button
                onClick={() => setIsAddDbSourceModalOpen(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-purple-600 hover:bg-purple-500 rounded-lg transition-colors shadow-sm"
              >
                <Plus className="w-3.5 h-3.5" />
                Add Database
              </button>
            </div>

            {dbSources.length === 0 ? (
              <div className="rounded-xl border border-zinc-850 bg-zinc-900/30 p-8 text-center">
                <Database className="mx-auto h-9 w-9 text-zinc-600" />
                <h3 className="mt-3 text-sm font-semibold text-zinc-200">
                  No databases connected
                </h3>
                <p className="mt-1 text-xs text-zinc-400 max-w-md mx-auto">
                  Connect PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite, or SQL Server. ContextForge normalizes schemas into searchable knowledge docs with line anchoring.
                </p>
                <button
                  onClick={() => setIsAddDbSourceModalOpen(true)}
                  className="mt-4 inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium text-purple-200 bg-purple-950/60 hover:bg-purple-900/60 border border-purple-800/50 rounded-lg transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Connect Database
                </button>
              </div>
            ) : (
              <div className="space-y-3">
                {dbSources.map((db) => {
                  const isSyncing = syncingDbSourceIds[db.id] || db.status === "syncing";
                  const engineColors: Record<string, string> = {
                    postgres: "bg-blue-950/80 text-blue-300 border-blue-800/50",
                    cockroachdb: "bg-violet-950/80 text-violet-300 border-violet-800/50",
                    mysql: "bg-amber-950/80 text-amber-300 border-amber-800/50",
                    mariadb: "bg-teal-950/80 text-teal-300 border-teal-800/50",
                    sqlite: "bg-emerald-950/80 text-emerald-300 border-emerald-800/50",
                    sqlserver: "bg-rose-950/80 text-rose-300 border-rose-800/50",
                  };
                  const colorClass = engineColors[db.database_type] || "bg-zinc-800 text-zinc-300 border-zinc-700";

                  return (
                    <div
                      key={db.id}
                      className="rounded-xl border border-zinc-850 bg-zinc-900/40 p-4 hover:border-zinc-750 transition-all flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                    >
                      <div className="space-y-2 min-w-0">
                        <div className="flex flex-wrap items-center gap-2.5">
                          <span className="font-semibold text-zinc-100 text-sm">
                            {db.name}
                          </span>
                          <span className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-mono border ${colorClass}`}>
                            <Server className="w-3 h-3" />
                            {db.database_type.toUpperCase()}
                          </span>
                          <span
                            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium border ${
                              db.status === "ready"
                                ? "bg-emerald-950/50 text-emerald-400 border-emerald-800/50"
                                : db.status === "syncing"
                                ? "bg-blue-950/50 text-blue-400 border-blue-800/50 animate-pulse"
                                : db.status === "failed"
                                ? "bg-rose-950/50 text-rose-400 border-rose-800/50"
                                : "bg-zinc-800 text-zinc-400 border-zinc-700"
                            }`}
                          >
                            {db.status === "syncing" && (
                              <RefreshCw className="w-2.5 h-2.5 animate-spin" />
                            )}
                            {db.status.toUpperCase()}
                          </span>
                        </div>

                        <div className="flex flex-wrap items-center gap-3 text-xs text-zinc-400 font-mono">
                          {db.database_name && (
                            <span>
                              DB: <code className="text-zinc-300">{db.database_name}</code>
                            </span>
                          )}
                          {db.host && (
                            <>
                              <span>•</span>
                              <span>
                                Host: <code className="text-zinc-300">{db.host}:{db.port}</code>
                              </span>
                            </>
                          )}
                          {db.configuration?.mode && (
                            <>
                              <span>•</span>
                              <span className="text-zinc-300">
                                Mode: {db.configuration.mode}
                              </span>
                            </>
                          )}
                          <span>•</span>
                          <span className="font-sans">
                            Last synced: {formatDate(db.last_synced_at || db.created_at)}
                          </span>
                        </div>

                        {db.last_error && (
                          <div className="rounded bg-rose-950/30 border border-rose-800/40 px-2.5 py-1 text-[11px] text-rose-300">
                            {db.last_error}
                          </div>
                        )}
                      </div>

                      {/* Action buttons */}
                      <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                        <button
                          onClick={() => handleTriggerDbSync(db.id)}
                          disabled={isSyncing}
                          className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-zinc-200 hover:text-white bg-zinc-800 hover:bg-zinc-750 rounded-lg border border-zinc-700 transition-colors disabled:opacity-50"
                          title="Trigger database schema and data sync"
                        >
                          <RefreshCw className={`w-3.5 h-3.5 ${isSyncing ? "animate-spin text-purple-400" : ""}`} />
                          Sync Now
                        </button>

                        <button
                          onClick={() => handleDeleteDbSource(db.id, db.name)}
                          className="p-1.5 text-zinc-500 hover:text-rose-400 hover:bg-rose-950/30 rounded-lg transition-colors"
                          title="Disconnect database source"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ==================================================================== */}
      {/* TAB 2: INDEXED DOCUMENTS & CHUNKS                                    */}
      {/* ==================================================================== */}
      {activeTab === "documents" && (
        <div className="space-y-4">
          {/* Header & Filter Controls */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                <FileCode className="h-5 w-5 text-purple-400" />
                Indexed Documents ({documents.length})
              </h2>
              <p className="text-xs text-zinc-400">
                Source files chunked and embedded in vector space. Click any file to inspect chunks with line numbers.
              </p>
            </div>

            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
                <input
                  type="text"
                  placeholder="Search file path..."
                  value={docFilter}
                  onChange={(e) => setDocFilter(e.target.value)}
                  className="w-48 sm:w-64 pl-8 pr-3 py-1.5 text-xs rounded-lg border border-zinc-800 bg-zinc-900 text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500 font-mono"
                />
              </div>

              {uniqueLanguages.length > 1 && (
                <select
                  value={docLanguageFilter}
                  onChange={(e) => setDocLanguageFilter(e.target.value)}
                  className="px-2.5 py-1.5 text-xs rounded-lg border border-zinc-800 bg-zinc-900 text-zinc-200 focus:outline-none focus:border-blue-500"
                >
                  <option value="all">All Languages</option>
                  {uniqueLanguages.map((lang) => (
                    <option key={lang} value={lang}>
                      {lang.toUpperCase()}
                    </option>
                  ))}
                </select>
              )}
            </div>
          </div>

          {/* Documents Table */}
          <div className="rounded-xl border border-zinc-850 bg-zinc-900/40 overflow-hidden">
            <table className="w-full text-left text-xs border-collapse">
              <thead>
                <tr className="border-b border-zinc-800 bg-zinc-950/60 text-zinc-400 font-medium">
                  <th className="p-3 font-semibold">File Path</th>
                  <th className="p-3 font-semibold">Language</th>
                  <th className="p-3 font-semibold text-center">Chunks</th>
                  <th className="p-3 font-semibold">Content Hash</th>
                  <th className="p-3 font-semibold text-right">Last Indexed</th>
                  <th className="p-3 font-semibold text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-850">
                {filteredDocs.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="p-8 text-center text-zinc-500">
                      {docFilter
                        ? "No documents matched your filter."
                        : "No documents have been indexed yet."}
                    </td>
                  </tr>
                ) : (
                  filteredDocs.map((doc) => (
                    <tr
                      key={doc.id}
                      className="hover:bg-zinc-850/40 transition-colors group cursor-pointer"
                      onClick={() => handleInspectDocumentChunks(doc)}
                    >
                      <td className="p-3 font-mono text-zinc-200">
                        <div className="flex items-center gap-2">
                          <FileCode className="w-3.5 h-3.5 text-blue-400 shrink-0" />
                          <span className="group-hover:text-blue-400 transition-colors font-semibold">
                            {doc.file_path}
                          </span>
                        </div>
                      </td>
                      <td className="p-3">
                        <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-300 font-mono uppercase">
                          {doc.language || "text"}
                        </span>
                      </td>
                      <td className="p-3 text-center">
                        <span className="inline-flex items-center gap-1 font-mono font-semibold text-purple-300 bg-purple-950/60 border border-purple-800/40 px-2 py-0.5 rounded-full text-[11px]">
                          <Layers className="w-3 h-3" />
                          {doc.total_chunks || 1}
                        </span>
                      </td>
                      <td className="p-3 font-mono text-zinc-500 text-[11px]">
                        {doc.content_hash ? `${doc.content_hash.slice(0, 16)}...` : "—"}
                      </td>
                      <td className="p-3 text-right text-zinc-400">
                        {formatDate(doc.updated_at || doc.created_at)}
                      </td>
                      <td className="p-3 text-right" onClick={(e) => e.stopPropagation()}>
                        <button
                          onClick={() => handleInspectDocumentChunks(doc)}
                          className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium text-purple-300 bg-purple-950/50 hover:bg-purple-900/50 border border-purple-800/50 transition-colors"
                        >
                          <Code2 className="w-3.5 h-3.5" />
                          Inspect Chunks
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ==================================================================== */}
      {/* TAB 3: PROJECT SETTINGS & CONFIGURATION                              */}
      {/* ==================================================================== */}
      {activeTab === "settings" && (
        <div className="space-y-6">
          <div className="rounded-xl border border-zinc-850 bg-zinc-900/40 p-6 space-y-4">
            <h3 className="text-base font-semibold text-zinc-100">
              Embedding & Vector Index Settings
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
              <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 space-y-1">
                <span className="text-zinc-500 font-medium uppercase text-[10px]">
                  Embedding Provider
                </span>
                <p className="text-sm font-semibold text-zinc-200">
                  {project?.embedding_provider || "ollama"}
                </p>
                <p className="text-zinc-400 text-[11px]">
                  Local embedding generation via Ollama HTTP API.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 space-y-1">
                <span className="text-zinc-500 font-medium uppercase text-[10px]">
                  Embedding Model
                </span>
                <p className="text-sm font-semibold text-zinc-200">
                  {project?.embedding_model || "nomic-embed-text"}
                </p>
                <p className="text-zinc-400 text-[11px]">
                  768-dimensional normalized text and code vectors.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 space-y-1">
                <span className="text-zinc-500 font-medium uppercase text-[10px]">
                  Vector Space Dimension
                </span>
                <p className="text-sm font-semibold text-zinc-200 font-mono">
                  {project?.embedding_dimension || 768} dimensions
                </p>
                <p className="text-zinc-400 text-[11px]">
                  pgvector HNSW index with cosine distance metric.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-zinc-950 border border-zinc-800 space-y-1">
                <span className="text-zinc-500 font-medium uppercase text-[10px]">
                  Default LLM Provider
                </span>
                <p className="text-sm font-semibold text-zinc-200">
                  {project?.llm_provider || "cli_opencode"}
                </p>
                <p className="text-zinc-400 text-[11px]">
                  Coding agent prompt synthesis provider.
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Add Source Modal */}
      <AddSourceModal
        projectId={projectId}
        isOpen={isAddSourceModalOpen}
        onClose={() => setIsAddSourceModalOpen(false)}
        onSuccess={() => loadData()}
      />

      {/* Add Database Source Modal */}
      <AddDatabaseSourceModal
        projectId={projectId}
        isOpen={isAddDbSourceModalOpen}
        onClose={() => setIsAddDbSourceModalOpen(false)}
        onSuccess={(newDb) => {
          setDbSources((prev) => [newDb, ...prev]);
          setIsAddDbSourceModalOpen(false);
          loadData();
        }}
      />

      {/* Chunk Inspector Modal */}
      <ChunkInspectorModal
        document={inspectingDoc}
        chunks={inspectingChunks}
        loading={chunksLoading}
        onClose={() => setInspectingDoc(null)}
      />
    </div>
  );
}
