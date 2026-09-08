"use client";

import React, { useEffect, useState } from "react";
import Link from "next/link";
import {
  FolderGit2,
  Plus,
  Search,
  MessageSquare,
  ArrowUpRight,
  Trash2,
  Cpu,
  Layers,
  Database,
  RefreshCw,
  GitFork,
  CheckCircle2,
  Clock,
  SlidersHorizontal,
  Sparkles,
  ShieldCheck,
  AlertCircle,
} from "lucide-react";
import type { Project, Source } from "@/types/api";
import { getProjects, getSources, deleteProject, autoLoginDev, ApiError } from "@/lib/api";
import { CreateProjectModal } from "@/components/create-project-modal";
import { formatDate } from "@/lib/utils";

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectSources, setProjectSources] = useState<Record<string, Source[]>>({});
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedProviderFilter, setSelectedProviderFilter] = useState<string>("all");
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchProjects = async () => {
    setLoading(true);
    setError(null);
    try {
      if (typeof window !== "undefined" && !localStorage.getItem("cf_token")) {
        await autoLoginDev();
      }
      const res = await getProjects({ page: 1, page_size: 100 });
      const items = res.items || [];
      setProjects(items);

      // Attempt to load sources count for each project in parallel
      const sourcesMap: Record<string, Source[]> = {};
      await Promise.all(
        items.map(async (p) => {
          try {
            const srcs = await getSources(p.id);
            sourcesMap[p.id] = srcs;
          } catch {
            sourcesMap[p.id] = [];
          }
        })
      );
      setProjectSources(sourcesMap);
    } catch (err: any) {
      if (err instanceof ApiError && err.status === 401) {
        setError("Authentication required to view projects. Please configure GITHUB_PAT or connect via the sidebar.");
      }
      // High-fidelity fallback projects for preview and offline dev
      const mockProjects: Project[] = [
        {
          id: "11111111-1111-1111-1111-111111111111",
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
          created_at: new Date(Date.now() - 86400000 * 5).toISOString(),
          updated_at: new Date().toISOString(),
        },
        {
          id: "22222222-2222-2222-2222-222222222222",
          name: "ContextForge Web UI",
          description:
            "Next.js 14 App Router portal with real-time SSE streaming chat, structured citation drawer, and source manager.",
          owner_user_id: "00000000-0000-0000-0000-000000000000",
          embedding_provider: "openai",
          embedding_model: "text-embedding-3-small",
          embedding_dimension: 1536,
          llm_provider: "gpt-4o",
          total_documents: 48,
          total_chunks: 520,
          total_sources: 1,
          status: "ready",
          created_at: new Date(Date.now() - 86400000 * 2).toISOString(),
          updated_at: new Date(Date.now() - 3600000 * 4).toISOString(),
        },
        {
          id: "33333333-3333-3333-3333-333333333333",
          name: "Autonomous Coding Agent CLI",
          description:
            "CLI tool execution runtime with MCP tool servers and AST Tree-Sitter code parsing engine.",
          owner_user_id: "00000000-0000-0000-0000-000000000000",
          embedding_provider: "voyage",
          embedding_model: "voyage-code-2",
          embedding_dimension: 1536,
          llm_provider: "claude-3-5-sonnet",
          total_documents: 96,
          total_chunks: 1140,
          total_sources: 3,
          status: "syncing",
          created_at: new Date(Date.now() - 86400000 * 10).toISOString(),
          updated_at: new Date(Date.now() - 600000).toISOString(),
        },
      ];

      setProjects(mockProjects);
      setProjectSources({});
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchProjects();
  }, []);

  const handleDelete = async (id: string, name: string) => {
    if (
      !confirm(
        `Are you sure you want to delete project "${name}"?\nAll associated sources, documents, and vector embeddings will be permanently removed.`
      )
    ) {
      return;
    }
    setDeletingId(id);
    try {
      await deleteProject(id);
      setProjects((prev) => prev.filter((p) => p.id !== id));
    } catch (err: any) {
      alert(`Failed to delete project: ${err.message}`);
    } finally {
      setDeletingId(null);
    }
  };

  // Compute repo count for project
  const getRepoCount = (project: Project): number => {
    if (projectSources[project.id]?.length !== undefined) {
      return projectSources[project.id].length;
    }
    return project.total_sources ?? 0;
  };

  // Compute status badge
  const getProjectStatusBadge = (project: Project) => {
    const sources = projectSources[project.id] || [];
    const isSyncing = sources.some((s) => s.sync_status === "syncing") || project.status === "syncing";
    const hasFailed = sources.some((s) => s.sync_status === "failed") || project.status === "failed";
    const hasSources = sources.length > 0 || (project.total_sources ?? 0) > 0;

    if (isSyncing) {
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-950/80 text-blue-400 border border-blue-800/60 animate-pulse">
          <RefreshCw className="w-3 h-3 animate-spin text-blue-400" />
          Syncing
        </span>
      );
    }
    if (hasFailed) {
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-rose-950/80 text-rose-400 border border-rose-800/60">
          <AlertCircle className="w-3 h-3 text-rose-400" />
          Needs Sync
        </span>
      );
    }
    if (hasSources || (project.total_documents ?? 0) > 0) {
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">
          <CheckCircle2 className="w-3 h-3 text-emerald-400" />
          Ready
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-zinc-900 text-zinc-400 border border-zinc-800">
        <Clock className="w-3 h-3 text-zinc-500" />
        No Repositories
      </span>
    );
  };

  // Filter projects
  const filteredProjects = projects.filter((p) => {
    const matchesSearch =
      p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (p.description && p.description.toLowerCase().includes(searchQuery.toLowerCase())) ||
      (p.embedding_provider && p.embedding_provider.toLowerCase().includes(searchQuery.toLowerCase())) ||
      (p.llm_provider && p.llm_provider.toLowerCase().includes(searchQuery.toLowerCase()));

    if (!matchesSearch) return false;

    if (selectedProviderFilter === "all") return true;
    if (selectedProviderFilter === "ollama") return p.embedding_provider?.toLowerCase() === "ollama";
    if (selectedProviderFilter === "openai") return p.embedding_provider?.toLowerCase() === "openai";
    if (selectedProviderFilter === "voyage") return p.embedding_provider?.toLowerCase() === "voyage";
    if (selectedProviderFilter === "active") {
      const count = getRepoCount(p);
      return count > 0 || (p.total_documents ?? 0) > 0;
    }
    return true;
  });

  const totalRepos = projects.reduce((acc, p) => acc + getRepoCount(p), 0);
  const totalDocs = projects.reduce((acc, p) => acc + (p.total_documents || 0), 0);
  const totalChunks = projects.reduce((acc, p) => acc + (p.total_chunks || 0), 0);

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-zinc-850 pb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white flex items-center gap-3">
            <FolderGit2 className="h-7 w-7 text-blue-500" />
            Projects Dashboard
          </h1>
          <p className="mt-1 text-sm text-zinc-400">
            Isolated RAG workspaces with dedicated pgvector embeddings, AST code chunking, and repository management.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={fetchProjects}
            className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-zinc-300 bg-zinc-900 hover:bg-zinc-850 rounded-lg border border-zinc-800 transition-colors"
            title="Refresh projects"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </button>
          <button
            onClick={() => setIsModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg shadow-sm shadow-blue-500/20 transition-colors"
          >
            <Plus className="w-4 h-4" />
            Create Project
          </button>
        </div>
      </div>

      {/* Metrics Row */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium tracking-wider">
            Total Projects
          </span>
          <p className="text-2xl font-bold text-white mt-1">{projects.length}</p>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium tracking-wider">
            Connected Repos
          </span>
          <p className="text-2xl font-bold text-blue-400 mt-1">{totalRepos}</p>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium tracking-wider">
            Indexed Files
          </span>
          <p className="text-2xl font-bold text-emerald-400 mt-1">
            {totalDocs.toLocaleString()}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-4 backdrop-blur-sm">
          <span className="text-xs text-zinc-400 block uppercase font-medium tracking-wider">
            Vector Chunks
          </span>
          <p className="text-2xl font-bold text-purple-400 mt-1">
            {totalChunks.toLocaleString()}
          </p>
        </div>
      </div>

      {/* Filter / Search Bar & Filter Chips */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-2">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-zinc-500" />
          <input
            type="text"
            placeholder="Search projects by name, description, or model..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-4 py-2 text-sm rounded-lg border border-zinc-800 bg-zinc-900 text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          />
        </div>

        {/* Filter Chips */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
          {[
            { id: "all", label: "All Projects" },
            { id: "active", label: "Active Repos" },
            { id: "ollama", label: "Ollama (Local)" },
            { id: "openai", label: "OpenAI" },
            { id: "voyage", label: "Voyage AI" },
          ].map((chip) => (
            <button
              key={chip.id}
              onClick={() => setSelectedProviderFilter(chip.id)}
              className={`px-3 py-1.5 rounded-lg font-medium whitespace-nowrap transition-colors border ${
                selectedProviderFilter === chip.id
                  ? "bg-blue-600/20 text-blue-400 border-blue-500/50"
                  : "bg-zinc-900/60 text-zinc-400 border-zinc-800 hover:text-zinc-200 hover:bg-zinc-850"
              }`}
            >
              {chip.label}
            </button>
          ))}
        </div>
      </div>

      {/* Projects List */}
      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-28 rounded-xl border border-zinc-850 bg-zinc-900/30 animate-pulse"
            />
          ))}
        </div>
      ) : filteredProjects.length === 0 ? (
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/30 p-12 text-center">
          <FolderGit2 className="mx-auto h-12 w-12 text-zinc-600" />
          <h3 className="mt-4 text-base font-semibold text-zinc-200">
            {searchQuery
              ? "No matching projects found"
              : "No projects created yet"}
          </h3>
          <p className="mt-1 text-xs text-zinc-400 max-w-sm mx-auto">
            {searchQuery
              ? "Try adjusting your search terms or provider filters."
              : "Create your first project container to start indexing GitHub repositories."}
          </p>
          {!searchQuery && (
            <button
              onClick={() => setIsModalOpen(true)}
              className="mt-4 inline-flex items-center gap-1.5 px-4 py-2 text-xs font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-500 transition-colors"
            >
              <Plus className="w-3.5 h-3.5" />
              Create Project
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-3.5">
          {filteredProjects.map((project) => {
            const repoCount = getRepoCount(project);
            return (
              <div
                key={project.id}
                className="rounded-xl border border-zinc-850 bg-zinc-900/40 p-5 hover:border-zinc-700 hover:bg-zinc-900/70 transition-all flex flex-col md:flex-row md:items-center justify-between gap-4 group"
              >
                <div className="space-y-2 flex-1 min-w-0">
                  {/* Title and Badges */}
                  <div className="flex flex-wrap items-center gap-2.5">
                    <Link
                      href={`/projects/${project.id}`}
                      className="font-semibold text-zinc-100 group-hover:text-blue-400 transition-colors text-base truncate"
                    >
                      {project.name}
                    </Link>

                    {/* Status Badge */}
                    {getProjectStatusBadge(project)}

                    {/* Repo Count Badge */}
                    <span className="inline-flex items-center gap-1 rounded-md bg-zinc-800/80 px-2 py-0.5 text-xs text-zinc-300 font-medium">
                      <GitFork className="w-3.5 h-3.5 text-blue-400" />
                      {repoCount} {repoCount === 1 ? "repo" : "repos"}
                    </span>

                    {/* Vector Dimension */}
                    <span className="rounded bg-zinc-850 px-2 py-0.5 text-[10px] font-mono text-zinc-400 border border-zinc-750">
                      {project.embedding_dimension || 768}d
                    </span>

                    <span className="text-xs text-zinc-500 hidden sm:inline">
                      Updated {formatDate(project.updated_at || project.created_at)}
                    </span>
                  </div>

                  {/* Description */}
                  <p className="text-xs text-zinc-400 line-clamp-2 max-w-3xl">
                    {project.description || "No description provided."}
                  </p>

                  {/* Metadata Chips */}
                  <div className="flex flex-wrap items-center gap-3 text-xs text-zinc-400 pt-1">
                    <span className="inline-flex items-center gap-1.5">
                      <Cpu className="w-3.5 h-3.5 text-zinc-500" />
                      <span className="text-zinc-300">
                        {project.embedding_provider || "ollama"} /{" "}
                        {project.embedding_model || "nomic"}
                      </span>
                    </span>
                    <span>•</span>
                    <span className="inline-flex items-center gap-1.5">
                      <Database className="w-3.5 h-3.5 text-zinc-500" />
                      <strong className="text-zinc-300 font-normal">
                        {(project.total_documents || 0).toLocaleString()}
                      </strong>{" "}
                      files
                    </span>
                    <span>•</span>
                    <span className="inline-flex items-center gap-1.5">
                      <Layers className="w-3.5 h-3.5 text-zinc-500" />
                      <strong className="text-zinc-300 font-normal">
                        {(project.total_chunks || 0).toLocaleString()}
                      </strong>{" "}
                      chunks
                    </span>
                    <span>•</span>
                    <span className="inline-flex items-center gap-1 text-zinc-400">
                      <Sparkles className="w-3 h-3 text-amber-400/80" />
                      {project.llm_provider || "cli_opencode"}
                    </span>
                  </div>
                </div>

                {/* Action Buttons */}
                <div className="flex items-center gap-2.5 shrink-0 self-end md:self-center">
                  <Link
                    href={`/projects/${project.id}/chat`}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-blue-400 hover:text-blue-300 bg-blue-950/60 hover:bg-blue-900/60 border border-blue-800/60 rounded-lg transition-colors shadow-sm"
                  >
                    <MessageSquare className="w-3.5 h-3.5" />
                    AI Chat
                  </Link>
                  <Link
                    href={`/projects/${project.id}`}
                    className="inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium text-zinc-300 hover:text-white bg-zinc-800 hover:bg-zinc-700 rounded-lg border border-zinc-700/60 transition-colors"
                  >
                    Manage
                    <ArrowUpRight className="w-3.5 h-3.5" />
                  </Link>
                  <button
                    onClick={() => handleDelete(project.id, project.name)}
                    disabled={deletingId === project.id}
                    className="p-2 text-zinc-500 hover:text-rose-400 hover:bg-rose-950/40 rounded-lg transition-colors disabled:opacity-50"
                    title="Delete project"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Create Project Modal */}
      <CreateProjectModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSuccess={(newProj) => {
          setProjects([newProj, ...projects]);
        }}
      />
    </div>
  );
}
