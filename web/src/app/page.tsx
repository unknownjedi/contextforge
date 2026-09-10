"use client";

import React, { useEffect, useState } from "react";
import Link from "next/link";
import {
  FolderGit2,
  Database,
  Layers,
  Sparkles,
  Plus,
  ArrowRight,
  MessageSquare,
  Activity,
  Cpu,
  RefreshCw,
  Key,
  X,
  Loader2,
} from "lucide-react";
import type { Project } from "@/types/api";
import { getProjects, autoLoginDev, loginWithPat, ApiError } from "@/lib/api";
import { CreateProjectModal } from "@/components/create-project-modal";
import { formatDate } from "@/lib/utils";

export default function DashboardPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [authRequired, setAuthRequired] = useState(false);
  const [isPatModalOpen, setIsPatModalOpen] = useState(false);
  const [patInput, setPatInput] = useState("");
  const [patLoading, setPatLoading] = useState(false);
  const [patError, setPatError] = useState<string | null>(null);

  const fetchProjects = async () => {
    setLoading(true);
    setError(null);
    setAuthRequired(false);
    try {
      if (typeof window !== "undefined" && !localStorage.getItem("cf_token")) {
        await autoLoginDev();
      }
      const res = await getProjects({ page: 1, page_size: 50 });
      setProjects(res.items || []);
    } catch (err: any) {
      console.warn("Failed to fetch projects:", err);
      if (err instanceof ApiError && err.status === 401) {
        setAuthRequired(true);
        setError(
          "Authentication required: Please configure GITHUB_PAT in .env or connect your GitHub Personal Access Token."
        );
      } else {
        setError(
          "Could not connect to ContextForge backend on localhost:8080. Please ensure the backend server is running."
        );
      }
      setProjects([]);
    } finally {
      setLoading(false);
    }
  };

  const handlePatSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!patInput.trim()) return;
    setPatLoading(true);
    setPatError(null);
    try {
      await loginWithPat(patInput.trim());
      setIsPatModalOpen(false);
      setPatInput("");
      fetchProjects();
    } catch (err: any) {
      setPatError(err.message || "Failed to authenticate PAT with GitHub.");
    } finally {
      setPatLoading(false);
    }
  };

  useEffect(() => {
    fetchProjects();
  }, []);

  const totalDocuments = projects.reduce(
    (acc, p) => acc + (p.total_documents || 0),
    0
  );
  const totalChunks = projects.reduce(
    (acc, p) => acc + (p.total_chunks || 0),
    0
  );

  return (
    <div className="space-y-8 max-w-7xl mx-auto">
      {/* Header Banner */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-zinc-850 pb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl flex items-center gap-3">
            ContextForge Dashboard
            <span className="text-xs px-2.5 py-0.5 rounded-full bg-blue-950 text-blue-400 border border-blue-800/60 font-mono font-normal">
              Engine v1.0
            </span>
          </h1>
          <p className="mt-1.5 text-sm text-zinc-400">
            Isolated RAG workspaces, vector embeddings, and real-time context streaming for AI agents.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={fetchProjects}
            className="flex items-center gap-2 px-3 py-2 text-xs font-medium text-zinc-300 bg-zinc-900 hover:bg-zinc-850 rounded-lg border border-zinc-800 transition-colors"
            title="Refresh dashboard"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} />
            Sync
          </button>
          <button
            onClick={() => setIsModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg shadow-sm shadow-blue-500/20 transition-colors"
          >
            <Plus className="w-4 h-4" />
            New Project
          </button>
        </div>
      </div>

      {authRequired ? (
        <div className="rounded-xl border border-blue-800/50 bg-blue-950/40 p-4 text-xs text-blue-200 flex flex-col sm:flex-row sm:items-center justify-between gap-3 shadow-lg">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-blue-900/80 text-blue-300 border border-blue-700/60">
              <Key className="h-4 w-4 text-amber-400" />
            </div>
            <div>
              <p className="font-semibold text-white">Authentication Required</p>
              <p className="text-zinc-400 text-[11px] mt-0.5">
                Set <code className="text-amber-300 font-mono bg-zinc-900 px-1 py-0.5 rounded">GITHUB_PAT</code> in your <code className="text-zinc-300 font-mono">.env</code> file, or connect your Personal Access Token.
              </p>
            </div>
          </div>
          <button
            onClick={() => setIsPatModalOpen(true)}
            className="self-start sm:self-auto shrink-0 flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg shadow-sm transition-colors"
          >
            <Key className="w-3.5 h-3.5 text-amber-300" />
            <span>Connect GitHub PAT</span>
          </button>
        </div>
      ) : error ? (
        <div className="rounded-xl border border-amber-800/40 bg-amber-950/30 p-4 text-xs text-amber-300/90 flex items-center justify-between">
          <span>{error}</span>
          <span className="text-[11px] text-amber-400/70 font-mono">Backend: offline (preview mode)</span>
        </div>
      ) : null}

      {/* Quick Stats Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="rounded-xl border border-zinc-850 bg-zinc-900/60 p-5 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-wider text-zinc-400">
              Active Projects
            </span>
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-950/80 text-blue-400 border border-blue-800/40">
              <FolderGit2 className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-3 text-3xl font-bold text-white tracking-tight">
            {projects.length}
          </p>
          <p className="mt-1 text-xs text-zinc-400">Isolated workspace containers</p>
        </div>

        <div className="rounded-xl border border-zinc-850 bg-zinc-900/60 p-5 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-wider text-zinc-400">
              Indexed Documents
            </span>
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-950/80 text-emerald-400 border border-emerald-800/40">
              <Database className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-3 text-3xl font-bold text-white tracking-tight">
            {totalDocuments.toLocaleString()}
          </p>
          <p className="mt-1 text-xs text-zinc-400">Source files parsed & chunked</p>
        </div>

        <div className="rounded-xl border border-zinc-850 bg-zinc-900/60 p-5 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-wider text-zinc-400">
              Vector Chunks
            </span>
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-950/80 text-purple-400 border border-purple-800/40">
              <Layers className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-3 text-3xl font-bold text-white tracking-tight">
            {totalChunks.toLocaleString()}
          </p>
          <p className="mt-1 text-xs text-zinc-400">768-dim embeddings stored</p>
        </div>

        <div className="rounded-xl border border-zinc-850 bg-zinc-900/60 p-5 backdrop-blur-sm">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-wider text-zinc-400">
              Context Query Engine
            </span>
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-amber-950/80 text-amber-400 border border-amber-800/40">
              <Sparkles className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-3 text-3xl font-bold text-emerald-400 tracking-tight flex items-center gap-2">
            Ready
          </p>
          <p className="mt-1 text-xs text-zinc-400">SSE Streaming & Citations</p>
        </div>
      </div>

      {/* Projects Section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-white">Active Projects</h2>
            <p className="text-xs text-zinc-400">
              Select a project to explore repositories, inspect documents, or query via RAG.
            </p>
          </div>
          <Link
            href="/projects"
            className="text-xs font-medium text-blue-400 hover:text-blue-300 flex items-center gap-1"
          >
            View All Projects
            <ArrowRight className="h-3.5 w-3.5" />
          </Link>
        </div>

        {loading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {[1, 2, 3].map((i) => (
              <div
                key={i}
                className="h-48 rounded-xl border border-zinc-850 bg-zinc-900/30 animate-pulse"
              />
            ))}
          </div>
        ) : projects.length === 0 ? (
          <div className="rounded-xl border border-zinc-850 bg-zinc-900/30 p-12 text-center">
            <FolderGit2 className="mx-auto h-12 w-12 text-zinc-600" />
            <h3 className="mt-4 text-base font-semibold text-zinc-200">
              No projects created yet
            </h3>
            <p className="mt-1 text-xs text-zinc-400 max-w-sm mx-auto">
              Get started by creating a new isolated project container and connecting your GitHub repositories.
            </p>
            <button
              onClick={() => setIsModalOpen(true)}
              className="mt-5 inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg"
            >
              <Plus className="w-4 h-4" />
              Create First Project
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {projects.map((project) => (
              <div
                key={project.id}
                className="group relative flex flex-col justify-between rounded-xl border border-zinc-850 bg-zinc-900/50 p-5 hover:border-blue-500/50 hover:bg-zinc-900/80 transition-all shadow-sm"
              >
                <div>
                  <div className="flex items-start justify-between gap-3">
                    <h3 className="font-semibold text-zinc-100 group-hover:text-blue-400 transition-colors">
                      {project.name}
                    </h3>
                    <span className="rounded bg-zinc-800 px-2 py-0.5 text-[10px] font-mono text-zinc-400 shrink-0">
                      {project.embedding_dimension || 768}d
                    </span>
                  </div>

                  <p className="mt-2 text-xs text-zinc-400 line-clamp-2 min-h-[32px]">
                    {project.description || "No description provided."}
                  </p>

                  <div className="mt-4 flex flex-wrap gap-1.5 text-[11px]">
                    <span className="inline-flex items-center gap-1 rounded bg-zinc-850 px-2 py-0.5 text-zinc-300">
                      <Cpu className="w-3 h-3 text-zinc-400" />
                      {project.embedding_provider || "ollama"}/
                      {project.embedding_model || "nomic"}
                    </span>
                    <span className="inline-flex items-center gap-1 rounded bg-zinc-850 px-2 py-0.5 text-zinc-300">
                      <Activity className="w-3 h-3 text-zinc-400" />
                      {project.llm_provider || "ollama"}
                    </span>
                  </div>
                </div>

                <div className="mt-6 pt-4 border-t border-zinc-800/80 flex items-center justify-between">
                  <div className="text-[11px] text-zinc-400">
                    <span className="font-medium text-zinc-300">
                      {project.total_documents || 0}
                    </span>{" "}
                    docs •{" "}
                    <span className="font-medium text-zinc-300">
                      {project.total_chunks || 0}
                    </span>{" "}
                    chunks
                  </div>
                  <div className="flex items-center gap-2">
                    <Link
                      href={`/projects/${project.id}/chat`}
                      className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-blue-400 hover:text-blue-300 bg-blue-950/60 hover:bg-blue-900/60 rounded-md border border-blue-800/50 transition-colors"
                    >
                      <MessageSquare className="w-3.5 h-3.5" />
                      Chat
                    </Link>
                    <Link
                      href={`/projects/${project.id}`}
                      className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-zinc-300 hover:text-white bg-zinc-800 hover:bg-zinc-700 rounded-md transition-colors"
                    >
                      Overview
                    </Link>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <CreateProjectModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSuccess={(newProj) => setProjects([newProj, ...projects])}
      />

      {/* Dashboard PAT Modal */}
      {isPatModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in">
          <div className="w-full max-w-sm rounded-xl border border-zinc-800 bg-zinc-950 p-5 shadow-2xl">
            <div className="flex items-center justify-between pb-3 mb-3 border-b border-zinc-800">
              <h3 className="text-sm font-semibold text-zinc-200 flex items-center gap-2">
                <Key className="h-4 w-4 text-amber-400" />
                Connect GitHub PAT
              </h3>
              <button
                onClick={() => setIsPatModalOpen(false)}
                className="text-zinc-400 hover:text-zinc-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <p className="text-xs text-zinc-400 mb-3">
              Enter your GitHub Personal Access Token to authenticate your local session.
            </p>
            {patError && (
              <div className="mb-3 rounded-lg bg-rose-950/50 border border-rose-800/60 p-2.5 text-xs text-rose-300">
                {patError}
              </div>
            )}
            <form onSubmit={handlePatSubmit} className="space-y-3">
              <input
                type="password"
                placeholder="ghp_..."
                value={patInput}
                onChange={(e) => setPatInput(e.target.value)}
                disabled={patLoading}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-200 focus:outline-none focus:border-blue-500 disabled:opacity-50 font-mono"
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setIsPatModalOpen(false)}
                  className="px-3 py-1.5 text-xs text-zinc-400 hover:bg-zinc-900 rounded-lg"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={patLoading || !patInput.trim()}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded-lg"
                >
                  {patLoading ? (
                    <>
                      <Loader2 className="w-3 h-3 animate-spin" />
                      <span>Validating...</span>
                    </>
                  ) : (
                    "Authenticate"
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
