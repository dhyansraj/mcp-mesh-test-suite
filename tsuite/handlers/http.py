"""
HTTP request handler.

Executes HTTP requests and captures responses.

Step configurations:

GET request:
    handler: http
    method: GET
    url: http://localhost:3000/health
    timeout: 30
    capture: response

POST request with JSON body:
    handler: http
    method: POST
    url: http://localhost:3000/api/chat
    headers:
      Content-Type: application/json
      Authorization: Bearer ${env:API_KEY}
    body:
      message: "Hello"
      model: "gpt-4"
    capture: response

Check status code:
    handler: http
    method: GET
    url: http://localhost:3000/health
    expect_status: 200
"""

import json

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure

# Import requests lazily to avoid issues if not installed
requests = None


def _get_requests():
    global requests
    if requests is None:
        import requests as req
        requests = req
    return requests


def execute(step: dict, context: dict) -> StepResult:
    """Execute an HTTP request."""
    req = _get_requests()

    method = step.get("method", "GET").upper()
    url = step.get("url")

    if not url:
        return failure("HTTP handler requires 'url' parameter")

    headers = step.get("headers", {})
    body = step.get("body")
    timeout = step.get("timeout", 30)
    expect_status = step.get("expect_status")

    try:
        # Make request based on method
        if method == "GET":
            response = req.get(url, headers=headers, timeout=timeout)
        elif method == "POST":
            if body and isinstance(body, dict):
                response = req.post(url, json=body, headers=headers, timeout=timeout)
            else:
                response = req.post(url, data=body, headers=headers, timeout=timeout)
        elif method == "PUT":
            if body and isinstance(body, dict):
                response = req.put(url, json=body, headers=headers, timeout=timeout)
            else:
                response = req.put(url, data=body, headers=headers, timeout=timeout)
        elif method == "DELETE":
            response = req.delete(url, headers=headers, timeout=timeout)
        elif method == "PATCH":
            if body and isinstance(body, dict):
                response = req.patch(url, json=body, headers=headers, timeout=timeout)
            else:
                response = req.patch(url, data=body, headers=headers, timeout=timeout)
        elif method == "HEAD":
            response = req.head(url, headers=headers, timeout=timeout)
        else:
            return failure(f"Unknown HTTP method: {method}")

        # Check expected status if specified
        status_ok = True
        if expect_status is not None:
            if isinstance(expect_status, list):
                status_ok = response.status_code in expect_status
            else:
                status_ok = response.status_code == expect_status

        # Determine success (2xx or expected status)
        is_success = status_ok and (response.status_code < 400 or expect_status is not None)

        return StepResult(
            exit_code=0 if is_success else 1,
            stdout=response.text,
            stderr=f"Status: {response.status_code}",
            success=is_success,
            error=None if is_success else f"HTTP {response.status_code}: {response.reason}",
        )

    except req.Timeout:
        return failure(f"Request timeout after {timeout}s", exit_code=124)
    except req.ConnectionError as e:
        return failure(f"Connection error: {e}")
    except Exception as e:
        return failure(f"HTTP request failed: {e}")
