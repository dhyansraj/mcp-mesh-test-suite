"""
Routine loading and execution.

Routines are reusable step sequences defined at different scopes:
- global: Available to all tests
- UC-level: Available to tests in that use case
- TC-level: Available to that specific test

Resolution order (most specific wins):
1. TC-level routines
2. UC-level routines
3. Global routines
"""

from dataclasses import dataclass
from typing import Any

from .context import runtime
from .discovery import RoutineSet


@dataclass
class ResolvedRoutine:
    """A routine resolved from a reference."""
    name: str
    scope: str
    description: str
    params_schema: dict[str, dict]
    steps: list[dict]


class RoutineResolver:
    """
    Resolves routine references to their definitions.

    Supports:
    - "routine_name" -> searches TC -> UC -> global
    - "global.routine_name" -> only global
    - "uc.routine_name" -> only UC-level
    - "tc.routine_name" -> only TC-level
    """

    def __init__(self, routine_sets: dict[str, RoutineSet]):
        """
        Initialize with discovered routine sets.

        Args:
            routine_sets: Dict mapping scope to RoutineSet
        """
        self.routine_sets = routine_sets

        # Register with runtime context
        for scope, rs in routine_sets.items():
            runtime.set_routines(scope, rs.routines)

    def resolve(self, reference: str, test_uc: str, test_tc: str) -> ResolvedRoutine | None:
        """
        Resolve a routine reference.

        Args:
            reference: Routine reference (e.g., "install_meshctl", "global.cleanup")
            test_uc: Current test's use case (e.g., "uc01_scaffolding")
            test_tc: Current test's ID (e.g., "uc01_scaffolding/tc01_python_agent")

        Returns:
            ResolvedRoutine or None if not found
        """
        # Check for explicit scope prefix
        if reference.startswith("global."):
            name = reference[7:]
            return self._get_routine("global", name)

        if reference.startswith("uc."):
            name = reference[3:]
            return self._get_routine(test_uc, name)

        if reference.startswith("tc."):
            name = reference[3:]
            return self._get_routine(test_tc, name)

        # No prefix: search TC -> UC -> global
        # TC-level
        routine = self._get_routine(test_tc, reference)
        if routine:
            return routine

        # UC-level
        routine = self._get_routine(test_uc, reference)
        if routine:
            return routine

        # Global
        routine = self._get_routine("global", reference)
        if routine:
            return routine

        return None

    def _get_routine(self, scope: str, name: str) -> ResolvedRoutine | None:
        """Get a routine from a specific scope."""
        rs = self.routine_sets.get(scope)
        if not rs or name not in rs.routines:
            return None

        routine_def = rs.routines[name]
        return ResolvedRoutine(
            name=name,
            scope=scope,
            description=routine_def.get("description", ""),
            params_schema=routine_def.get("params", {}),
            steps=routine_def.get("steps", []),
        )

    def validate_params(self, routine: ResolvedRoutine, params: dict) -> list[str]:
        """
        Validate parameters against routine's schema.

        Returns list of error messages (empty if valid).
        """
        errors = []

        for param_name, schema in routine.params_schema.items():
            required = schema.get("required", False)
            param_type = schema.get("type", "string")

            if param_name not in params:
                if required:
                    errors.append(f"Missing required parameter: {param_name}")
                continue

            value = params[param_name]

            # Type checking
            type_map = {
                "string": str,
                "str": str,
                "integer": int,
                "int": int,
                "number": (int, float),
                "float": float,
                "boolean": bool,
                "bool": bool,
            }

            expected_type = type_map.get(param_type)
            if expected_type and not isinstance(value, expected_type):
                errors.append(
                    f"Parameter '{param_name}' should be {param_type}, "
                    f"got {type(value).__name__}"
                )

        return errors

    def apply_defaults(self, routine: ResolvedRoutine, params: dict) -> dict:
        """Apply default values from schema to params."""
        result = dict(params)

        for param_name, schema in routine.params_schema.items():
            if param_name not in result and "default" in schema:
                result[param_name] = schema["default"]

        return result


def create_routine_context(
    routine: ResolvedRoutine,
    params: dict,
    parent_context: dict,
) -> dict:
    """
    Create execution context for a routine.

    Merges parent context with routine-specific params.
    """
    return {
        **parent_context,
        "params": params,
        "routine": {
            "name": routine.name,
            "scope": routine.scope,
        },
    }
