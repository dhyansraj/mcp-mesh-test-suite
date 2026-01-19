"""
LLM handler for testing LLM provider integrations.

Supports Claude (Anthropic) and OpenAI providers with:
- Semantic response validation (keywords, patterns)
- Token usage verification
- Rate limit handling with backoff
- Mock mode for fast/free testing

Step configurations:

Claude provider:
    handler: llm
    provider: claude
    model: claude-3-sonnet-20240229
    prompt: "What is 2 + 2?"
    expect_contains:
      - "4"
      - "four"
    capture: response

OpenAI provider:
    handler: llm
    provider: openai
    model: gpt-4
    prompt: "Explain quantum computing"
    expect_pattern: "qubit|superposition|entanglement"
    max_tokens: 500
    capture: response

With system message:
    handler: llm
    provider: claude
    model: claude-3-haiku-20240307
    system: "You are a helpful math tutor."
    prompt: "What is the square root of 144?"
    expect_contains: ["12"]

Token usage validation:
    handler: llm
    provider: openai
    model: gpt-3.5-turbo
    prompt: "Say hello"
    max_input_tokens: 100
    max_output_tokens: 50

Mock mode (set TSUITE_MOCK_LLM=true):
    handler: llm
    provider: claude
    prompt: "Any prompt"
    mock_response: "Mocked response for testing"
"""

import json
import os
import re
import time
from typing import Optional

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure

# Lazy imports for LLM libraries
anthropic_client = None
openai_client = None


def _get_anthropic():
    """Lazy load Anthropic client."""
    global anthropic_client
    if anthropic_client is None:
        try:
            import anthropic
            api_key = os.environ.get("ANTHROPIC_API_KEY")
            if not api_key:
                return None
            anthropic_client = anthropic.Anthropic(api_key=api_key)
        except ImportError:
            return None
    return anthropic_client


def _get_openai():
    """Lazy load OpenAI client."""
    global openai_client
    if openai_client is None:
        try:
            import openai
            api_key = os.environ.get("OPENAI_API_KEY")
            if not api_key:
                return None
            openai_client = openai.OpenAI(api_key=api_key)
        except ImportError:
            return None
    return openai_client


def _is_mock_mode() -> bool:
    """Check if mock mode is enabled."""
    return os.environ.get("TSUITE_MOCK_LLM", "").lower() in ("true", "1", "yes")


def _call_claude(
    model: str,
    prompt: str,
    system: Optional[str] = None,
    max_tokens: int = 1024,
    temperature: float = 0.0,
) -> dict:
    """Call Claude API."""
    client = _get_anthropic()
    if client is None:
        raise RuntimeError("Anthropic client not available. Check ANTHROPIC_API_KEY and anthropic package.")

    messages = [{"role": "user", "content": prompt}]

    kwargs = {
        "model": model,
        "max_tokens": max_tokens,
        "temperature": temperature,
        "messages": messages,
    }

    if system:
        kwargs["system"] = system

    # Retry with exponential backoff for rate limits
    for attempt in range(3):
        try:
            response = client.messages.create(**kwargs)
            return {
                "content": response.content[0].text if response.content else "",
                "model": response.model,
                "input_tokens": response.usage.input_tokens,
                "output_tokens": response.usage.output_tokens,
                "stop_reason": response.stop_reason,
            }
        except Exception as e:
            error_str = str(e)
            if "rate_limit" in error_str.lower() or "429" in error_str:
                if attempt < 2:
                    wait_time = (2 ** attempt) * 5  # 5s, 10s, 20s
                    time.sleep(wait_time)
                    continue
            raise


def _call_openai(
    model: str,
    prompt: str,
    system: Optional[str] = None,
    max_tokens: int = 1024,
    temperature: float = 0.0,
) -> dict:
    """Call OpenAI API."""
    client = _get_openai()
    if client is None:
        raise RuntimeError("OpenAI client not available. Check OPENAI_API_KEY and openai package.")

    messages = []
    if system:
        messages.append({"role": "system", "content": system})
    messages.append({"role": "user", "content": prompt})

    # Retry with exponential backoff for rate limits
    for attempt in range(3):
        try:
            response = client.chat.completions.create(
                model=model,
                messages=messages,
                max_tokens=max_tokens,
                temperature=temperature,
            )
            choice = response.choices[0]
            return {
                "content": choice.message.content or "",
                "model": response.model,
                "input_tokens": response.usage.prompt_tokens if response.usage else 0,
                "output_tokens": response.usage.completion_tokens if response.usage else 0,
                "stop_reason": choice.finish_reason,
            }
        except Exception as e:
            error_str = str(e)
            if "rate_limit" in error_str.lower() or "429" in error_str:
                if attempt < 2:
                    wait_time = (2 ** attempt) * 5
                    time.sleep(wait_time)
                    continue
            raise


def _get_mock_response(step: dict) -> dict:
    """Generate mock response for testing."""
    mock_response = step.get("mock_response", "This is a mock LLM response for testing.")

    # If expect_contains is set, include those in the mock response
    expect_contains = step.get("expect_contains", [])
    if expect_contains and mock_response == "This is a mock LLM response for testing.":
        mock_response = f"Mock response containing: {', '.join(expect_contains)}"

    return {
        "content": mock_response,
        "model": f"mock-{step.get('provider', 'unknown')}",
        "input_tokens": 10,
        "output_tokens": len(mock_response.split()),
        "stop_reason": "end_turn",
    }


def _validate_response(response: dict, step: dict) -> tuple[bool, list[str]]:
    """
    Validate LLM response against expectations.

    Returns (passed, list of error messages)
    """
    errors = []
    content = response.get("content", "").lower()

    # Check expect_contains (any keyword match)
    expect_contains = step.get("expect_contains", [])
    if expect_contains:
        found_any = False
        for keyword in expect_contains:
            if keyword.lower() in content:
                found_any = True
                break
        if not found_any:
            errors.append(f"Response does not contain any of: {expect_contains}")

    # Check expect_contains_all (all keywords must match)
    expect_contains_all = step.get("expect_contains_all", [])
    if expect_contains_all:
        missing = []
        for keyword in expect_contains_all:
            if keyword.lower() not in content:
                missing.append(keyword)
        if missing:
            errors.append(f"Response missing required keywords: {missing}")

    # Check expect_pattern (regex match)
    expect_pattern = step.get("expect_pattern")
    if expect_pattern:
        if not re.search(expect_pattern, response.get("content", ""), re.IGNORECASE):
            errors.append(f"Response does not match pattern: {expect_pattern}")

    # Check expect_not_contains
    expect_not_contains = step.get("expect_not_contains", [])
    for keyword in expect_not_contains:
        if keyword.lower() in content:
            errors.append(f"Response should not contain: {keyword}")

    # Check token limits
    max_input_tokens = step.get("max_input_tokens")
    if max_input_tokens and response.get("input_tokens", 0) > max_input_tokens:
        errors.append(f"Input tokens ({response['input_tokens']}) exceeded limit ({max_input_tokens})")

    max_output_tokens = step.get("max_output_tokens")
    if max_output_tokens and response.get("output_tokens", 0) > max_output_tokens:
        errors.append(f"Output tokens ({response['output_tokens']}) exceeded limit ({max_output_tokens})")

    return len(errors) == 0, errors


def execute(step: dict, context: dict) -> StepResult:
    """Execute an LLM request."""
    provider = step.get("provider", "claude").lower()
    prompt = step.get("prompt")

    if not prompt:
        return failure("LLM handler requires 'prompt' parameter")

    # Get parameters
    model = step.get("model")
    system = step.get("system")
    max_tokens = step.get("max_tokens", 1024)
    temperature = step.get("temperature", 0.0)

    # Set default models if not specified
    if not model:
        if provider == "claude":
            model = "claude-3-haiku-20240307"
        elif provider == "openai":
            model = "gpt-3.5-turbo"
        else:
            return failure(f"Unknown provider: {provider}")

    # Check for mock mode
    if _is_mock_mode():
        try:
            response = _get_mock_response(step)
            passed, errors = _validate_response(response, step)

            output = json.dumps({
                "mode": "mock",
                "provider": provider,
                "model": response["model"],
                "content": response["content"],
                "input_tokens": response["input_tokens"],
                "output_tokens": response["output_tokens"],
                "validation": {
                    "passed": passed,
                    "errors": errors,
                },
            }, indent=2)

            if not passed:
                return failure(
                    f"Mock response validation failed: {'; '.join(errors)}",
                    stdout=output,
                )

            return success(stdout=output)

        except Exception as e:
            return failure(f"Mock mode error: {e}")

    # Real API call
    try:
        if provider == "claude":
            response = _call_claude(
                model=model,
                prompt=prompt,
                system=system,
                max_tokens=max_tokens,
                temperature=temperature,
            )
        elif provider == "openai":
            response = _call_openai(
                model=model,
                prompt=prompt,
                system=system,
                max_tokens=max_tokens,
                temperature=temperature,
            )
        else:
            return failure(f"Unknown provider: {provider}. Supported: claude, openai")

        # Validate response
        passed, errors = _validate_response(response, step)

        output = json.dumps({
            "mode": "live",
            "provider": provider,
            "model": response["model"],
            "content": response["content"],
            "input_tokens": response["input_tokens"],
            "output_tokens": response["output_tokens"],
            "stop_reason": response["stop_reason"],
            "validation": {
                "passed": passed,
                "errors": errors,
            },
        }, indent=2)

        if not passed:
            return failure(
                f"Response validation failed: {'; '.join(errors)}",
                stdout=output,
            )

        return success(stdout=output)

    except RuntimeError as e:
        # Missing API key or client
        return failure(str(e))
    except Exception as e:
        return failure(f"LLM request failed: {e}")
