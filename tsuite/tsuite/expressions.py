"""
Expression language for assertions and variable resolution.

Supports:
- Variable resolution: ${var}, ${json:$.path}, ${file:/path}, etc.
- Operators: ==, !=, contains, matches, exists, >, <, >=, <=, length
- Config/state/captured variable access
"""

import re
import os
import json
from pathlib import Path
from typing import Any

from jsonpath_ng import parse as jsonpath_parse


class ExpressionError(Exception):
    """Error during expression evaluation."""
    pass


class ExpressionEvaluator:
    """
    Evaluates expressions against a context.

    Context should include:
    - config: dict - Configuration values
    - state: dict - Shared state
    - captured: dict - Captured variables
    - last: dict - Last step result (exit_code, stdout, stderr)
    - workdir: Path - Working directory
    - fixtures_dir: Path - Fixtures directory
    """

    # Variable pattern: ${...}
    VAR_PATTERN = re.compile(r"\$\{([^}]+)\}")

    # Expression pattern: ${var} operator value
    EXPR_PATTERN = re.compile(
        r"^\$\{([^}]+)\}\s+"
        r"(==|!=|>=|<=|>|<|contains|matches|exists|not\s+exists|not\s+contains|is|length)\s*"
        r"(.*)$"
    )

    def __init__(self, context: dict):
        self.context = context

    def resolve_variable(self, var: str) -> Any:
        """
        Resolve a variable reference.

        Supported formats:
        - config.packages.cli_version -> config value
        - state.agent_port -> state value
        - captured.output -> captured variable
        - last.exit_code -> last step result
        - last.stdout -> last step stdout
        - json:$.path.to.field -> JSONPath on last.stdout
        - jsonfile:/path:$.query -> JSONPath on file
        - file:/path/to/file -> File contents
        - fixture:expected/foo.json -> Fixture file contents
        - env:VAR_NAME -> Environment variable
        - params.name -> Routine parameter (if in routine context)
        """
        # Handle prefixed variables
        if var.startswith("config."):
            return self._resolve_path(self.context.get("config", {}), var[7:])

        if var.startswith("state."):
            return self._resolve_path(self.context.get("state", {}), var[6:])

        if var.startswith("captured."):
            return self._resolve_path(self.context.get("captured", {}), var[9:])

        if var.startswith("last."):
            return self._resolve_path(self.context.get("last", {}), var[5:])

        if var.startswith("params."):
            return self._resolve_path(self.context.get("params", {}), var[7:])

        if var.startswith("json:"):
            # JSONPath on stdout
            path = var[5:]
            stdout = self.context.get("last", {}).get("stdout", "")
            try:
                data = json.loads(stdout)
                return self._jsonpath(data, path)
            except json.JSONDecodeError:
                return None

        if var.startswith("jsonfile:"):
            # ${jsonfile:/path:$.query}
            rest = var[9:]
            if ":" in rest:
                file_path, jpath = rest.split(":", 1)
            else:
                file_path, jpath = rest, "$"
            try:
                with open(file_path) as f:
                    data = json.load(f)
                return self._jsonpath(data, jpath)
            except (FileNotFoundError, json.JSONDecodeError):
                return None

        if var.startswith("file:"):
            path = var[5:]
            try:
                return Path(path).read_text()
            except FileNotFoundError:
                return None

        if var.startswith("fixture:"):
            fixture_name = var[8:]
            fixtures_dir = self.context.get("fixtures_dir")
            if fixtures_dir:
                path = Path(fixtures_dir) / fixture_name
                try:
                    return path.read_text()
                except FileNotFoundError:
                    return None
            return None

        if var.startswith("env:"):
            return os.environ.get(var[4:])

        # Try common paths without prefix
        # Check last first (most common in assertions)
        if var in ("exit_code", "stdout", "stderr"):
            return self.context.get("last", {}).get(var)

        # Then check captured
        if var in self.context.get("captured", {}):
            return self.context["captured"][var]

        # Then check state
        if var in self.context.get("state", {}):
            return self.context["state"][var]

        # Then check config
        return self._resolve_path(self.context.get("config", {}), var)

    def _resolve_path(self, obj: dict, path: str) -> Any:
        """Resolve a dot-notation path in a dictionary."""
        parts = path.split(".")
        value = obj
        for part in parts:
            if isinstance(value, dict) and part in value:
                value = value[part]
            else:
                return None
        return value

    def _jsonpath(self, data: Any, path: str) -> Any:
        """Execute JSONPath query on data."""
        try:
            expr = jsonpath_parse(path)
            matches = [m.value for m in expr.find(data)]
            if len(matches) == 0:
                return None
            if len(matches) == 1:
                return matches[0]
            return matches
        except Exception:
            return None

    def interpolate(self, text: str) -> str:
        """
        Interpolate all ${...} variables in a string.

        Returns the string with variables replaced by their values.
        """
        def replace(match):
            var = match.group(1)
            value = self.resolve_variable(var)
            return str(value) if value is not None else match.group(0)

        return self.VAR_PATTERN.sub(replace, text)

    def evaluate(self, expr: str) -> tuple[bool, str]:
        """
        Evaluate an assertion expression.

        Args:
            expr: Expression like "${exit_code} == 0"

        Returns:
            (passed, message) tuple
        """
        match = self.EXPR_PATTERN.match(expr.strip())
        if not match:
            return False, f"Invalid expression syntax: {expr}"

        var_name, operator, expected_raw = match.groups()
        operator = operator.strip().lower()

        # Resolve the variable
        actual = self.resolve_variable(var_name)

        # Handle operators that don't need an expected value
        if operator == "exists":
            passed = actual is not None
            return passed, f"{'exists' if passed else 'does not exist'}"

        if operator == "not exists":
            passed = actual is None
            return passed, f"{'does not exist' if passed else 'exists'}"

        # Parse expected value
        expected_raw = expected_raw.strip()

        # Remove quotes if present
        if (expected_raw.startswith("'") and expected_raw.endswith("'")) or \
           (expected_raw.startswith('"') and expected_raw.endswith('"')):
            expected = expected_raw[1:-1]
        else:
            expected = expected_raw

        # Interpolate variables in expected value
        expected = self.interpolate(expected)

        # Convert types for comparison
        if operator in ("==", "!=", ">", "<", ">=", "<="):
            # Try numeric comparison
            try:
                actual_num = float(actual) if actual is not None else None
                expected_num = float(expected)
                if actual_num is not None:
                    actual = actual_num
                    expected = expected_num
            except (ValueError, TypeError):
                pass

        # Execute operator
        if operator == "==":
            passed = actual == expected
            return passed, f"actual={repr(actual)}, expected={repr(expected)}"

        if operator == "!=":
            passed = actual != expected
            return passed, f"actual={repr(actual)}, should not equal {repr(expected)}"

        if operator == "contains":
            passed = expected in str(actual) if actual else False
            return passed, f"{'contains' if passed else 'does not contain'} {repr(expected)}"

        if operator == "not contains":
            passed = expected not in str(actual) if actual else True
            return passed, f"{'does not contain' if passed else 'contains'} {repr(expected)}"

        if operator == "matches":
            try:
                passed = bool(re.search(expected, str(actual))) if actual else False
                return passed, f"{'matches' if passed else 'does not match'} pattern {repr(expected)}"
            except re.error as e:
                return False, f"Invalid regex pattern: {e}"

        if operator == "is":
            type_map = {
                "string": str,
                "str": str,
                "number": (int, float),
                "int": int,
                "integer": int,
                "float": float,
                "bool": bool,
                "boolean": bool,
                "list": list,
                "array": list,
                "dict": dict,
                "object": dict,
                "null": type(None),
                "none": type(None),
            }
            expected_type = type_map.get(expected.lower())
            if expected_type is None:
                return False, f"Unknown type: {expected}"
            passed = isinstance(actual, expected_type)
            return passed, f"type is {type(actual).__name__}, expected {expected}"

        if operator == "length":
            # Parse "length > 5" style
            length_match = re.match(r"([><=!]+)\s*(\d+)", expected)
            if length_match:
                length_op, length_val = length_match.groups()
                actual_len = len(actual) if hasattr(actual, "__len__") else 0
                length_val = int(length_val)

                ops = {
                    ">": lambda a, b: a > b,
                    "<": lambda a, b: a < b,
                    ">=": lambda a, b: a >= b,
                    "<=": lambda a, b: a <= b,
                    "==": lambda a, b: a == b,
                    "!=": lambda a, b: a != b,
                }

                if length_op in ops:
                    passed = ops[length_op](actual_len, length_val)
                    return passed, f"length={actual_len}, expected {length_op} {length_val}"

            return False, f"Invalid length expression: {expected}"

        if operator in (">", "<", ">=", "<="):
            try:
                actual_num = float(actual) if actual is not None else 0
                expected_num = float(expected)
                ops = {
                    ">": lambda a, b: a > b,
                    "<": lambda a, b: a < b,
                    ">=": lambda a, b: a >= b,
                    "<=": lambda a, b: a <= b,
                }
                passed = ops[operator](actual_num, expected_num)
                return passed, f"actual={actual_num}, expected {operator} {expected_num}"
            except (ValueError, TypeError):
                return False, f"Cannot compare: {actual} {operator} {expected}"

        return False, f"Unknown operator: {operator}"
