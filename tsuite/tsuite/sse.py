"""
Server-Sent Events (SSE) event manager for real-time test updates.

Provides thread-safe event broadcasting to multiple subscribers.
Supports HTTP forwarding for subprocess -> server communication.
"""

import json
import os
import threading
import time
from collections import defaultdict
from dataclasses import dataclass, asdict
from datetime import datetime
from queue import Queue, Empty
from typing import Generator, Optional
import urllib.request
import urllib.error


@dataclass
class SSEEvent:
    """An SSE event with type and payload."""
    type: str
    data: dict
    timestamp: str = ""

    def __post_init__(self):
        if not self.timestamp:
            self.timestamp = datetime.utcnow().isoformat() + "Z"

    def to_sse(self) -> str:
        """Format as SSE message."""
        payload = {
            "type": self.type,
            "timestamp": self.timestamp,
            **self.data,
        }
        return f"data: {json.dumps(payload)}\n\n"


class SSEManager:
    """
    Thread-safe SSE event manager.

    Supports:
    - Multiple subscribers per run
    - Global event stream (all runs)
    - Automatic cleanup of disconnected clients
    - HTTP forwarding for subprocess communication
    """

    def __init__(self, event_server_url: Optional[str] = None):
        self._lock = threading.RLock()
        # run_id -> list of subscriber queues
        self._run_subscribers: dict[str, list[Queue]] = defaultdict(list)
        # Global subscribers (receive all events)
        self._global_subscribers: list[Queue] = []
        # Current run being executed
        self._current_run_id: Optional[str] = None
        # HTTP server URL for forwarding events (used in subprocess mode)
        self._event_server_url = event_server_url or os.environ.get("TSUITE_EVENT_SERVER")
        # Event cache for replaying events to late subscribers
        # run_id -> list of SSE formatted events
        self._event_cache: dict[str, list[str]] = defaultdict(list)
        # Maximum events to cache per run
        self._max_cache_size = 100

    def set_event_server(self, url: Optional[str]):
        """Set the event server URL for HTTP forwarding."""
        self._event_server_url = url

    def _forward_event_http(self, event_data: dict):
        """Forward event to the API server via HTTP."""
        if not self._event_server_url:
            return

        try:
            url = f"{self._event_server_url}/api/events/emit"
            data = json.dumps(event_data).encode("utf-8")
            req = urllib.request.Request(
                url,
                data=data,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=5) as response:
                pass  # Just send, don't care about response
        except (urllib.error.URLError, TimeoutError):
            # Log but don't fail - events are best-effort
            pass

    def subscribe_run(self, run_id: str) -> Queue:
        """Subscribe to events for a specific run."""
        queue = Queue()
        with self._lock:
            self._run_subscribers[run_id].append(queue)
        return queue

    def unsubscribe_run(self, run_id: str, queue: Queue):
        """Unsubscribe from run events."""
        with self._lock:
            if run_id in self._run_subscribers:
                try:
                    self._run_subscribers[run_id].remove(queue)
                except ValueError:
                    pass
                # Clean up empty subscriber lists
                if not self._run_subscribers[run_id]:
                    del self._run_subscribers[run_id]

    def subscribe_global(self) -> Queue:
        """Subscribe to all events."""
        queue = Queue()
        with self._lock:
            self._global_subscribers.append(queue)
        return queue

    def unsubscribe_global(self, queue: Queue):
        """Unsubscribe from global events."""
        with self._lock:
            try:
                self._global_subscribers.remove(queue)
            except ValueError:
                pass

    def emit(self, event: SSEEvent, run_id: Optional[str] = None):
        """
        Emit an event to subscribers.

        Args:
            event: The SSE event to emit
            run_id: Optional run_id to target specific subscribers
        """
        # If event server is configured, forward via HTTP (subprocess mode)
        if self._event_server_url:
            event_data = {
                "type": event.type,
                "timestamp": event.timestamp,
                **event.data,
            }
            if run_id:
                event_data["run_id"] = run_id
            self._forward_event_http(event_data)
            return  # Don't emit locally when forwarding

        # Local emission (server mode)
        sse_data = event.to_sse()

        with self._lock:
            # Cache event for late subscribers
            if run_id:
                cache = self._event_cache[run_id]
                cache.append(sse_data)
                # Trim cache if too large
                if len(cache) > self._max_cache_size:
                    self._event_cache[run_id] = cache[-self._max_cache_size:]

            # Send to run-specific subscribers
            if run_id and run_id in self._run_subscribers:
                for queue in self._run_subscribers[run_id]:
                    queue.put(sse_data)

            # Send to global subscribers
            for queue in self._global_subscribers:
                queue.put(sse_data)

    def set_current_run(self, run_id: Optional[str]):
        """Set the current run ID for context."""
        with self._lock:
            self._current_run_id = run_id

    def get_current_run(self) -> Optional[str]:
        """Get the current run ID."""
        with self._lock:
            return self._current_run_id

    def get_cached_events(self, run_id: str) -> list[str]:
        """Get cached events for a run (for replaying to late subscribers)."""
        with self._lock:
            return list(self._event_cache.get(run_id, []))

    def clear_cache(self, run_id: str):
        """Clear cached events for a run."""
        with self._lock:
            if run_id in self._event_cache:
                del self._event_cache[run_id]

    # Convenience methods for emitting specific event types

    def emit_run_started(self, run_id: str, total_tests: int):
        """Emit run_started event."""
        self.set_current_run(run_id)
        self.emit(
            SSEEvent(
                type="run_started",
                data={
                    "run_id": run_id,
                    "total_tests": total_tests,
                },
            ),
            run_id=run_id,
        )

    def emit_test_started(self, run_id: str, test_id: str, name: str):
        """Emit test_started event."""
        self.emit(
            SSEEvent(
                type="test_started",
                data={
                    "run_id": run_id,
                    "test_id": test_id,
                    "name": name,
                },
            ),
            run_id=run_id,
        )

    def emit_step_completed(
        self,
        run_id: str,
        test_id: str,
        step_index: int,
        phase: str,
        status: str,
        duration_ms: int,
        handler: Optional[str] = None,
    ):
        """Emit step_completed event."""
        self.emit(
            SSEEvent(
                type="step_completed",
                data={
                    "run_id": run_id,
                    "test_id": test_id,
                    "step_index": step_index,
                    "phase": phase,
                    "status": status,
                    "duration_ms": duration_ms,
                    "handler": handler,
                },
            ),
            run_id=run_id,
        )

    def emit_test_completed(
        self,
        run_id: str,
        test_id: str,
        status: str,
        duration_ms: int,
        passed: int = 0,
        failed: int = 0,
    ):
        """Emit test_completed event."""
        self.emit(
            SSEEvent(
                type="test_completed",
                data={
                    "run_id": run_id,
                    "test_id": test_id,
                    "status": status,
                    "duration_ms": duration_ms,
                    "steps_passed": passed,
                    "steps_failed": failed,
                },
            ),
            run_id=run_id,
        )

    def emit_run_completed(
        self,
        run_id: str,
        passed: int,
        failed: int,
        skipped: int = 0,
        duration_ms: int = 0,
    ):
        """Emit run_completed event."""
        self.emit(
            SSEEvent(
                type="run_completed",
                data={
                    "run_id": run_id,
                    "passed": passed,
                    "failed": failed,
                    "skipped": skipped,
                    "duration_ms": duration_ms,
                },
            ),
            run_id=run_id,
        )
        self.set_current_run(None)


def stream_events(
    queue: Queue,
    timeout: float = 30.0,
    keepalive_interval: float = 15.0,
) -> Generator[str, None, None]:
    """
    Generator that yields SSE events from a queue.

    Sends keepalive comments to prevent connection timeout.

    Args:
        queue: Queue to read events from
        timeout: Max time to wait for events before sending keepalive
        keepalive_interval: Interval for keepalive messages
    """
    last_event_time = time.time()

    while True:
        try:
            # Wait for event with timeout
            event = queue.get(timeout=keepalive_interval)
            yield event
            last_event_time = time.time()
        except Empty:
            # Send keepalive comment
            yield ": keepalive\n\n"


# Global SSE manager instance
sse_manager = SSEManager()
