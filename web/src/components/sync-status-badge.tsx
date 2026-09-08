import React from "react";
import type { SourceSyncStatus } from "@/types/api";
import { CheckCircle2, AlertCircle, Clock, RefreshCw, MinusCircle } from "lucide-react";
import { cn } from "@/lib/utils";

interface SyncStatusBadgeProps {
  status: SourceSyncStatus | string;
  className?: string;
}

export function SyncStatusBadge({ status, className }: SyncStatusBadgeProps) {
  switch (status) {
    case "synced":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/80 text-emerald-400 border border-emerald-800/60",
            className
          )}
        >
          <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
          Synced
        </span>
      );
    case "syncing":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-950/80 text-blue-400 border border-blue-800/60 animate-pulse",
            className
          )}
        >
          <RefreshCw className="w-3.5 h-3.5 text-blue-400 animate-spin" />
          Syncing
        </span>
      );
    case "queued":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/80 text-amber-400 border border-amber-800/60",
            className
          )}
        >
          <Clock className="w-3.5 h-3.5 text-amber-400" />
          Queued
        </span>
      );
    case "failed":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-rose-950/80 text-rose-400 border border-rose-800/60",
            className
          )}
        >
          <AlertCircle className="w-3.5 h-3.5 text-rose-400" />
          Failed
        </span>
      );
    case "idle":
    default:
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-zinc-900 text-zinc-400 border border-zinc-800",
            className
          )}
        >
          <MinusCircle className="w-3.5 h-3.5 text-zinc-400" />
          {status || "Idle"}
        </span>
      );
  }
}
