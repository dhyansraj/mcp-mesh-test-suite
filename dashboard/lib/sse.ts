/**
 * SSE (Server-Sent Events) hook for real-time updates
 */

"use client";

import { useEffect, useState, useCallback, useRef } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:9999";

export interface SSEEvent {
  type: string;
  timestamp: string;
  run_id?: string;
  test_id?: string;
  name?: string;
  status?: string;
  duration_ms?: number;
  total_tests?: number;
  passed?: number;
  failed?: number;
  skipped?: number;
  step_index?: number;
  phase?: string;
  handler?: string;
  steps_passed?: number;
  steps_failed?: number;
}

export interface UseSSEOptions {
  runId?: string;
  onEvent?: (event: SSEEvent) => void;
  maxEvents?: number;
}

export interface UseSSEResult {
  events: SSEEvent[];
  connected: boolean;
  currentRunId: string | null;
  error: Error | null;
  clearEvents: () => void;
}

export function useSSE(options: UseSSEOptions = {}): UseSSEResult {
  const { runId, onEvent, maxEvents = 100 } = options;
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [currentRunId, setCurrentRunId] = useState<string | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const clearEvents = useCallback(() => {
    setEvents([]);
  }, []);

  useEffect(() => {
    // Build URL based on whether we're subscribing to a specific run or global
    const url = runId
      ? `${API_BASE}/api/runs/${runId}/stream`
      : `${API_BASE}/api/events`;

    const eventSource = new EventSource(url);
    eventSourceRef.current = eventSource;

    eventSource.onopen = () => {
      setConnected(true);
      setError(null);
    };

    eventSource.onerror = () => {
      setConnected(false);
      setError(new Error("SSE connection lost"));
    };

    eventSource.onmessage = (e) => {
      try {
        const event: SSEEvent = JSON.parse(e.data);

        // Handle connected event
        if (event.type === "connected") {
          if ("current_run_id" in event) {
            setCurrentRunId((event as SSEEvent & { current_run_id: string | null }).current_run_id);
          }
          return;
        }

        // Track current run
        if (event.type === "run_started" && event.run_id) {
          setCurrentRunId(event.run_id);
        } else if (event.type === "run_completed") {
          setCurrentRunId(null);
        }

        // Add event to list
        setEvents((prev) => {
          const newEvents = [event, ...prev];
          // Limit number of stored events
          return newEvents.slice(0, maxEvents);
        });

        // Call callback if provided
        onEvent?.(event);
      } catch (err) {
        console.error("Failed to parse SSE event:", err);
      }
    };

    return () => {
      eventSource.close();
      eventSourceRef.current = null;
    };
  }, [runId, maxEvents, onEvent]);

  return {
    events,
    connected,
    currentRunId,
    error,
    clearEvents,
  };
}

// Hook for subscribing to a specific run's events
export function useRunStream(runId: string) {
  return useSSE({ runId });
}

// Hook for global event stream (live view)
export function useLiveEvents(options?: Omit<UseSSEOptions, "runId">) {
  return useSSE(options);
}
