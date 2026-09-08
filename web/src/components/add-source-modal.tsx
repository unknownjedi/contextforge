"use client";

import React, { useState, useEffect } from "react";
import type { CreateSourceRequest, SourceIngestionQueued, AuthMethod } from "@/types/api";
import { createSource } from "@/lib/api";
import { X, GitBranch, Loader2, Key, ShieldCheck, Github, Globe, UploadCloud } from "lucide-react";

interface AddSourceModalProps {
  projectId: string;
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (result: SourceIngestionQueued) => void;
}

export function AddSourceModal({
  projectId,
  isOpen,
  onClose,
  onSuccess,
}: AddSourceModalProps) {
  const [sourceType, setSourceType] = useState<"github" | "url" | "document">("github");
  const [repoUrl, setRepoUrl] = useState("");
  const [repoOwner, setRepoOwner] = useState("");
  const [repoName, setRepoName] = useState("");
  const [branch, setBranch] = useState("main");
  const [authMethod, setAuthMethod] = useState<AuthMethod>("oauth");
  const [patToken, setPatToken] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Auto-fill PAT from localStorage if available
  useEffect(() => {
    if (typeof window !== "undefined") {
      const savedPat = localStorage.getItem("cf_pat");
      if (savedPat) {
        setPatToken(savedPat);
      }
    }
  }, [isOpen]);

  // Parse repo owner and repo name from GitHub URL or shorthand
  const handleUrlChange = (value: string) => {
    setRepoUrl(value);
    const cleaned = value.trim().replace(/\/+$/, "");

    // Check if matches github.com/owner/repo or https://github.com/owner/repo
    const ghMatch = cleaned.match(/(?:github\.com\/|git@github\.com:)?([^/\s]+)\/([^/\s#]+)/);
    if (ghMatch) {
      setRepoOwner(ghMatch[1]);
      setRepoName(ghMatch[2].replace(/\.git$/, ""));
    }
  };

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    let owner = repoOwner.trim();
    let name = repoName.trim();

    if (!owner || !name) {
      if (repoUrl) {
        const ghMatch = repoUrl.trim().match(/(?:github\.com\/|git@github\.com:)?([^/\s]+)\/([^/\s#]+)/);
        if (ghMatch) {
          owner = ghMatch[1];
          name = ghMatch[2].replace(/\.git$/, "");
        }
      }
    }

    if (!owner || !name) {
      setError("Please provide a valid GitHub repository URL or specify owner and name.");
      return;
    }

    if (authMethod === "pat" && !patToken.trim()) {
      setError("Please enter a GitHub Personal Access Token (PAT) for PAT authentication.");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const payload: CreateSourceRequest = {
        name: `${owner}/${name}`,
        repo_url: repoUrl.trim() || `https://github.com/${owner}/${name}`,
        repo_owner: owner,
        repo_name: name,
        branch: branch.trim() || "main",
        auth_method: authMethod,
        pat_token: authMethod === "pat" ? patToken.trim() : undefined,
      };

      const result = await createSource(projectId, payload);
      onSuccess(result);
      onClose();
    } catch (err: any) {
      setError(err.message || "Failed to connect repository.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-lg rounded-xl border border-zinc-800 bg-zinc-950 p-6 shadow-2xl">
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-zinc-800 pb-4 mb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-950/60 border border-blue-800/50 text-blue-400">
              <Github className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-zinc-100">
                Connect Repository
              </h2>
              <p className="text-xs text-zinc-400">
                Attach a code repository to this isolated context container.
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Source Type Selector */}
        <div className="flex rounded-lg bg-zinc-900/90 p-1 border border-zinc-800 mb-4">
          <button
            type="button"
            onClick={() => setSourceType("github")}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "github"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <Github className="w-3.5 h-3.5" />
            <span>GitHub</span>
          </button>
          <button
            type="button"
            onClick={() => setSourceType("url")}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "url"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <Globe className="w-3.5 h-3.5" />
            <span>URL / Web</span>
            <span className="text-[9px] px-1 py-0.2 rounded bg-amber-950/80 text-amber-400 border border-amber-800/40 font-mono">
              Roadmap
            </span>
          </button>
          <button
            type="button"
            onClick={() => setSourceType("document")}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "document"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <UploadCloud className="w-3.5 h-3.5" />
            <span>Document</span>
            <span className="text-[9px] px-1 py-0.2 rounded bg-amber-950/80 text-amber-400 border border-amber-800/40 font-mono">
              Roadmap
            </span>
          </button>
        </div>

        {sourceType === "url" && (
          <div className="space-y-4 py-2">
            <div className="rounded-xl border border-blue-900/40 bg-blue-950/20 p-5 text-center space-y-3">
              <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-blue-900/60 border border-blue-700/60 text-blue-400">
                <Globe className="h-5 w-5" />
              </div>
              <div>
                <h4 className="text-sm font-semibold text-white">URL & Web Crawler Connector</h4>
                <p className="text-xs text-zinc-400 mt-1 max-w-sm mx-auto">
                  Automated scraping, sitemap traversal, and change-detection for API docs and web knowledge bases.
                </p>
              </div>
              <div className="inline-flex items-center gap-2 px-2.5 py-1 rounded-full bg-amber-950/60 text-amber-400 border border-amber-800/50 text-[11px] font-mono">
                <span>Scheduled for v1.1 release (Roadmap §30-33)</span>
              </div>
              <p className="text-[11px] text-zinc-500">
                Currently active engines: <strong>GitHub Repositories</strong> and <strong>Relational Databases</strong> (Postgres, MySQL, SQLite, MSSQL).
              </p>
            </div>
            <div className="flex justify-end pt-2 border-t border-zinc-850">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        )}

        {sourceType === "document" && (
          <div className="space-y-4 py-2">
            <div className="rounded-xl border border-blue-900/40 bg-blue-950/20 p-5 text-center space-y-3">
              <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-xl bg-blue-900/60 border border-blue-700/60 text-blue-400">
                <UploadCloud className="h-5 w-5" />
              </div>
              <div>
                <h4 className="text-sm font-semibold text-white">Manual Document Upload</h4>
                <p className="text-xs text-zinc-400 mt-1 max-w-sm mx-auto">
                  Drag-and-drop ingestion of Markdown files, PDFs, text specifications, and architecture documents.
                </p>
              </div>
              <div className="inline-flex items-center gap-2 px-2.5 py-1 rounded-full bg-amber-950/60 text-amber-400 border border-amber-800/50 text-[11px] font-mono">
                <span>Scheduled for v1.1 release (Roadmap §30-33)</span>
              </div>
              <p className="text-[11px] text-zinc-500">
                Currently active engines: <strong>GitHub Repositories</strong> and <strong>Relational Databases</strong> (Postgres, MySQL, SQLite, MSSQL).
              </p>
            </div>
            <div className="flex justify-end pt-2 border-t border-zinc-850">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Close
              </button>
            </div>
          </div>
        )}

        {sourceType === "github" && (
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300">
                {error}
              </div>
            )}

            {/* Repo URL Input */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Repository URL or Path <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                required
                placeholder="https://github.com/facebook/react or owner/repo"
                value={repoUrl}
                onChange={(e) => handleUrlChange(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
              />
            </div>

            {/* Owner & Repo Name (auto-parsed) */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                  Owner / Organization
                </label>
                <input
                  type="text"
                  placeholder="e.g. facebook"
                  value={repoOwner}
                  onChange={(e) => setRepoOwner(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                  Repository Name
                </label>
                <input
                  type="text"
                  placeholder="e.g. react"
                  value={repoName}
                  onChange={(e) => setRepoName(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
            </div>

            {/* Branch Input */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5 flex items-center gap-1.5">
                <GitBranch className="w-3.5 h-3.5 text-zinc-400" />
                Default Branch
              </label>
              <input
                type="text"
                placeholder="main"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
              />
            </div>

            {/* Authentication Method */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Authentication Method
              </label>
              <div className="grid grid-cols-2 gap-3">
                <button
                  type="button"
                  onClick={() => setAuthMethod("oauth")}
                  className={`flex items-center justify-center gap-2 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                    authMethod === "oauth"
                      ? "border-blue-500 bg-blue-600/15 text-blue-300"
                      : "border-zinc-800 bg-zinc-900/60 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <ShieldCheck className="h-4 w-4" />
                  GitHub App OAuth
                </button>
                <button
                  type="button"
                  onClick={() => setAuthMethod("pat")}
                  className={`flex items-center justify-center gap-2 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                    authMethod === "pat"
                      ? "border-blue-500 bg-blue-600/15 text-blue-300"
                      : "border-zinc-800 bg-zinc-900/60 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <Key className="h-4 w-4 text-amber-400" />
                  Personal Access Token
                </button>
              </div>
            </div>

            {/* PAT Input Field if authMethod === 'pat' */}
            {authMethod === "pat" && (
              <div className="space-y-1.5 animate-in fade-in slide-in-from-top-1 duration-150">
                <div className="flex items-center justify-between">
                  <label className="block text-xs font-medium text-zinc-300">
                    GitHub PAT Token <span className="text-rose-400">*</span>
                  </label>
                  {patToken && (
                    <button
                      type="button"
                      onClick={() => setPatToken("")}
                      className="text-[10px] text-zinc-500 hover:text-zinc-400"
                    >
                      Clear
                    </button>
                  )}
                </div>
                <input
                  type="password"
                  placeholder="ghp_..."
                  value={patToken}
                  onChange={(e) => setPatToken(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
                />
                <p className="text-[10px] text-zinc-500">
                  Requires <code className="text-zinc-400">repo:read</code> scope for private or public repositories.
                </p>
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-3 pt-4 border-t border-zinc-800">
              <button
                type="button"
                onClick={onClose}
                disabled={loading}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={loading}
                className="flex items-center gap-2 px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors disabled:opacity-50 shadow-sm shadow-blue-500/20"
              >
                {loading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                Connect & Ingest
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
