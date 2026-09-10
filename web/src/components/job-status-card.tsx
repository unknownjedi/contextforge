"use client";

import React, { useEffect, useState, useCallback, useRef } from "react";
import type { IngestionJob } from "@/types/api";
import { getJob } from "@/lib/api";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Activity,
  Layers,
  FileCheck2,
} from "lucide-react";
import { formatDate } from "@/lib/utils";

interface JobStatusCardProps {
  projectId: string;
  activeJob: IngestionJob | null;
  recentJobs?: IngestionJob[];
  onJobUpdated?: (job: IngestionJob) => void;
  onJobComplete?: (job: IngestionJob) => void;
}

export function JobStatusCard({
  projectId,
  activeJob,
  recentJobs = [],
  onJobUpdated,
  onJobComplete,
}: JobStatusCardProps) {
  const [currentJob, setCurrentJob] = useState<IngestionJob | null>(activeJob);
  const [isPolling, setIsPolling] = useState(false);
  const [allJobs, setAllJobs] = useState<IngestionJob[]>(recentJobs);
  const pollTimerRef = useRef<NodeJS.Timeout | null>(null);

  const getJobId = (job: IngestionJob | null | undefined): string => {
    if (!job) return "";
    return job.id || (job as any).job_id || "";
  };

  // Sync with prop when activeJob changes
  useEffect(() => {
    if (activeJob) {
      const activeId = getJobId(activeJob);
      const normalizedJob: IngestionJob = {
        ...activeJob,
        id: activeId || activeJob.id,
      };
      setCurrentJob(normalizedJob);
      setAllJobs((prev) => {
        const exists = prev.some((j) => getJobId(j) === activeId && activeId !== "");
        if (exists) {
          return prev.map((j) => (getJobId(j) === activeId ? normalizedJob : j));
        }
        return [normalizedJob, ...prev];
      });
    }
  }, [activeJob]);

  // Polling logic via API client
  const pollJobStatus = useCallback(
    async (jobId: string) => {
      try {
        const job = await getJob(jobId, projectId);
        setCurrentJob(job);
        onJobUpdated?.(job);

        setAllJobs((prev) =>
          prev.map((j) => (j.id === jobId ? job : j))
        );

        if (job.status === "completed" || job.status === "failed") {
          setIsPolling(false);
          onJobComplete?.(job);
          if (pollTimerRef.current) {
            clearInterval(pollTimerRef.current);
            pollTimerRef.current = null;
          }
        }
      } catch (err) {
        console.warn("Error polling job status:", err);
      }
    },
    [projectId, onJobUpdated, onJobComplete]
  );

  useEffect(() => {
    const jid = getJobId(currentJob);
    if (currentJob && jid && (currentJob.status === "running" || currentJob.status === "pending")) {
      setIsPolling(true);
      if (pollTimerRef.current) clearInterval(pollTimerRef.current);

      pollTimerRef.current = setInterval(() => {
        pollJobStatus(jid);
      }, 2000);

      return () => {
        if (pollTimerRef.current) {
          clearInterval(pollTimerRef.current);
          pollTimerRef.current = null;
        }
      };
    } else {
      setIsPolling(false);
    }
  }, [currentJob, pollJobStatus]);

  const primaryJob = currentJob || (allJobs.length > 0 ? allJobs[0] : null);

  if (!primaryJob && allJobs.length === 0) {
    return (
      <div className="rounded-xl border border-zinc-850 bg-zinc-900/40 p-5 backdrop-blur-sm">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-800 text-zinc-400">
              <Activity className="h-4 w-4" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-zinc-200">Ingestion Jobs</h3>
              <p className="text-xs text-zinc-500">
                Track asynchronous repository parsing and vector embedding jobs.
              </p>
            </div>
          </div>
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs text-zinc-400 bg-zinc-800/80 border border-zinc-700/60">
            <span className="w-1.5 h-1.5 rounded-full bg-zinc-500" />
            Idle
          </span>
        </div>
        <p className="mt-4 text-xs text-zinc-400">
          No ingestion jobs have been run recently. Trigger a sync on any source above to index new commits.
        </p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "running":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-950/80 text-blue-400 border border-blue-800/60 animate-pulse">
            <RefreshCw className="w-3 h-3 animate-spin text-blue-400" />
            Running
          </span>
        );
      case "pending":
      case "queued":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/80 text-amber-400 border border-amber-800/60">
            <Clock className="w-3 h-3 text-amber-400" />
            Queued
          </span>
        );
      case "completed":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">
            <CheckCircle2 className="w-3 h-3 text-emerald-400" />
            Completed
          </span>
        );
      case "failed":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-rose-950/80 text-rose-400 border border-rose-800/60">
            <AlertCircle className="w-3 h-3 text-rose-400" />
            Failed
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-zinc-800 text-zinc-400">
            {status}
          </span>
        );
    }
  };

  const processed = primaryJob?.processed_files ?? 0;
  const total = primaryJob?.total_files ?? 0;
  const progressPercent = primaryJob?.progress_percent ?? (total > 0 ? Math.round((processed / total) * 100) : 0);

  return (
    <div className="rounded-xl border border-zinc-850 bg-zinc-900/50 p-5 backdrop-blur-sm space-y-4">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-zinc-800/80">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-950/80 border border-blue-800/50 text-blue-400">
            <Activity className="h-5 w-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-zinc-100">
                Ingestion Jobs Status
              </h3>
              {isPolling && (
                <span className="inline-flex items-center gap-1 text-[10px] text-blue-400 font-mono bg-blue-950/60 px-1.5 py-0.5 rounded border border-blue-800/40">
                  <span className="w-1.5 h-1.5 rounded-full bg-blue-400 animate-ping" />
                  Live Polling (2s)
                </span>
              )}
            </div>
            <p className="text-xs text-zinc-400">
              Active AST parsing, code chunking, and pgvector embeddings.
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {primaryJob && getStatusBadge(primaryJob.status)}
          {primaryJob && (
            <button
              onClick={() => {
                const jid = getJobId(primaryJob);
                if (jid) pollJobStatus(jid);
              }}
              className="p-1.5 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded-lg transition-colors"
              title="Poll latest job status"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${isPolling ? "animate-spin" : ""}`} />
            </button>
          )}
        </div>
      </div>

      {/* Primary Job Display */}
      {primaryJob && (
        <div className="space-y-3">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 text-xs">
            <div className="flex items-center gap-2 text-zinc-300">
              <span className="text-zinc-500 font-mono">Job ID:</span>
              <code className="text-blue-400 font-mono">
                {(() => {
                  const jid = getJobId(primaryJob);
                  if (!jid) return "pending";
                  return jid.length >= 12 ? `${jid.slice(0, 8)}...${jid.slice(-4)}` : jid;
                })()}
              </code>
            </div>
            <div className="flex items-center gap-3 text-zinc-400 font-mono text-[11px]">
              <span className="flex items-center gap-1">
                <FileCheck2 className="w-3.5 h-3.5 text-zinc-500" />
                {processed} / {total} files processed
              </span>
              <span>•</span>
              <span className="font-bold text-zinc-200">
                {progressPercent}%
              </span>
            </div>
          </div>

          {/* Progress Bar */}
          <div className="w-full bg-zinc-800/80 rounded-full h-2.5 overflow-hidden p-0.5 border border-zinc-700/50">
            <div
              className={`h-full transition-all duration-300 rounded-full ${
                primaryJob.status === "failed"
                  ? "bg-rose-500"
                  : primaryJob.status === "completed"
                  ? "bg-emerald-500"
                  : "bg-blue-500 animate-pulse"
              }`}
              style={{ width: `${Math.max(progressPercent, 4)}%` }}
            />
          </div>

          {/* Error message banner if failed */}
          {primaryJob.error_message && (
            <div className="rounded-lg bg-rose-950/40 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
              <div>
                <strong className="font-semibold">Ingestion Failure:</strong>{" "}
                {primaryJob.error_message}
              </div>
            </div>
          )}

          {/* Timing details */}
          <div className="flex flex-wrap items-center justify-between gap-2 text-[11px] text-zinc-500 pt-1">
            <span>Started: {formatDate(primaryJob.created_at)}</span>
            {primaryJob.finished_at && (
              <span>Finished: {formatDate(primaryJob.finished_at)}</span>
            )}
          </div>
        </div>
      )}

      {/* Recent Jobs History list (if more than 1) */}
      {allJobs.length > 1 && (
        <div className="pt-3 border-t border-zinc-800/60 space-y-2">
          <span className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 block">
            Recent Jobs History
          </span>
          <div className="space-y-1.5 max-h-36 overflow-y-auto">
            {allJobs.slice(1, 4).map((job) => (
              <div
                key={job.id}
                className="flex items-center justify-between p-2 rounded-lg bg-zinc-950/50 border border-zinc-850 text-xs"
              >
                <div className="flex items-center gap-2">
                  <Layers className="w-3.5 h-3.5 text-zinc-500" />
                  <code className="text-zinc-300 font-mono text-[11px]">
                    {getJobId(job).slice(0, 8) || "job"}
                  </code>
                  <span className="text-zinc-500 text-[10px]">
                    {formatDate(job.created_at)}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-zinc-400 font-mono text-[11px]">
                    {job.processed_files ?? 0}/{job.total_files ?? 0}
                  </span>
                  {getStatusBadge(job.status)}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
