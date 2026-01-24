"""
YAML utilities with comment preservation using ruamel.yaml.

This module provides functions for reading and writing YAML files
while preserving comments, which is essential for the test case editor.
"""

from pathlib import Path
from typing import Any, Dict, Optional, Union
from ruamel.yaml import YAML
from ruamel.yaml.comments import CommentedMap, CommentedSeq
import io


def create_yaml_instance() -> YAML:
    """Create a YAML instance configured for round-trip (comment preservation)."""
    yaml = YAML(typ='rt')  # Round-trip mode preserves comments
    yaml.preserve_quotes = True
    yaml.default_flow_style = False
    # indent: mapping indent, sequence indent, offset for sequence item content
    yaml.indent(mapping=2, sequence=4, offset=2)
    yaml.width = 1000  # Prevent line wrapping
    return yaml


def load_yaml_file(file_path: Union[str, Path]) -> CommentedMap:
    """
    Load a YAML file preserving comments.

    Args:
        file_path: Path to the YAML file

    Returns:
        CommentedMap containing the YAML data with comments preserved
    """
    yaml = create_yaml_instance()
    path = Path(file_path)

    if not path.exists():
        raise FileNotFoundError(f"YAML file not found: {file_path}")

    with open(path, 'r', encoding='utf-8') as f:
        data = yaml.load(f)

    return data if data is not None else CommentedMap()


def save_yaml_file(data: Union[CommentedMap, Dict], file_path: Union[str, Path]) -> None:
    """
    Save data to a YAML file preserving comments.

    Args:
        data: YAML data (CommentedMap for comment preservation, or regular dict)
        file_path: Path to save the YAML file
    """
    yaml = create_yaml_instance()
    path = Path(file_path)

    # Ensure parent directory exists
    path.parent.mkdir(parents=True, exist_ok=True)

    with open(path, 'w', encoding='utf-8') as f:
        yaml.dump(data, f)


def yaml_to_string(data: Union[CommentedMap, Dict]) -> str:
    """
    Convert YAML data to string preserving comments.

    Args:
        data: YAML data

    Returns:
        YAML string representation
    """
    yaml = create_yaml_instance()
    stream = io.StringIO()
    yaml.dump(data, stream)
    return stream.getvalue()


def string_to_yaml(content: str) -> CommentedMap:
    """
    Parse YAML string preserving comments.

    Args:
        content: YAML string

    Returns:
        CommentedMap containing the parsed data
    """
    yaml = create_yaml_instance()
    data = yaml.load(content)
    return data if data is not None else CommentedMap()


def update_yaml_field(data: CommentedMap, field: str, value: Any) -> None:
    """
    Update a field in the YAML data while preserving structure.

    Args:
        data: CommentedMap to update
        field: Field name to update
        value: New value
    """
    data[field] = value


def convert_to_commented_map(data: Dict) -> CommentedMap:
    """
    Convert a regular dict to CommentedMap recursively.

    Args:
        data: Regular dictionary

    Returns:
        CommentedMap equivalent
    """
    if isinstance(data, dict):
        result = CommentedMap()
        for key, value in data.items():
            result[key] = convert_to_commented_map(value)
        return result
    elif isinstance(data, list):
        result = CommentedSeq()
        for item in data:
            result.append(convert_to_commented_map(item))
        return result
    else:
        return data


def merge_yaml_updates(original: CommentedMap, updates: Dict, delete_marker: str = "__DELETE__") -> CommentedMap:
    """
    Merge updates into original YAML data, preserving comments in original.

    This is useful when you want to update specific fields from a JSON/dict
    while keeping the original YAML structure and comments intact.

    Args:
        original: Original CommentedMap with comments
        updates: Dictionary with updates to apply
        delete_marker: Special string value that signals field deletion

    Returns:
        Updated CommentedMap with preserved comments
    """
    keys_to_delete = []

    for key, value in updates.items():
        # Check for delete marker
        if value == delete_marker:
            keys_to_delete.append(key)
            continue

        if key in original:
            if isinstance(value, dict) and isinstance(original[key], CommentedMap):
                # Recursively merge nested dicts
                merge_yaml_updates(original[key], value, delete_marker)
            elif isinstance(value, list):
                # For lists, replace but convert to CommentedSeq
                original[key] = convert_to_commented_map(value)
            else:
                # Direct value update
                original[key] = value
        else:
            # New key - add it
            original[key] = convert_to_commented_map(value)

    # Delete marked keys
    for key in keys_to_delete:
        if key in original:
            del original[key]

    return original


def get_test_case_structure(data: CommentedMap) -> Dict[str, Any]:
    """
    Extract test case structure as a regular dict for JSON serialization.

    This converts the CommentedMap to a regular dict suitable for
    sending to the frontend via JSON API.

    Args:
        data: CommentedMap containing test case data

    Returns:
        Regular dict with test case structure
    """
    def to_dict(obj):
        if isinstance(obj, (CommentedMap, dict)):
            return {k: to_dict(v) for k, v in obj.items()}
        elif isinstance(obj, (CommentedSeq, list)):
            return [to_dict(item) for item in obj]
        else:
            return obj

    return to_dict(data)


# Test case specific helpers

def get_step_by_index(data: CommentedMap, phase: str, index: int) -> Optional[CommentedMap]:
    """
    Get a step by phase and index.

    Args:
        data: Test case YAML data
        phase: Phase name (pre_run, test, post_run)
        index: Step index

    Returns:
        Step data or None if not found
    """
    if phase not in data:
        return None
    steps = data[phase]
    if not isinstance(steps, (list, CommentedSeq)) or index >= len(steps):
        return None
    return steps[index]


def update_step(data: CommentedMap, phase: str, index: int, updates: Dict) -> bool:
    """
    Update a step in place.

    Args:
        data: Test case YAML data
        phase: Phase name
        index: Step index
        updates: Fields to update

    Returns:
        True if successful, False if step not found
    """
    step = get_step_by_index(data, phase, index)
    if step is None:
        return False

    merge_yaml_updates(step, updates)
    return True


def add_step(data: CommentedMap, phase: str, step: Dict, index: Optional[int] = None) -> None:
    """
    Add a new step to a phase.

    Args:
        data: Test case YAML data
        phase: Phase name
        step: Step data
        index: Optional index to insert at (appends if None)
    """
    if phase not in data:
        data[phase] = CommentedSeq()

    new_step = convert_to_commented_map(step)

    if index is None:
        data[phase].append(new_step)
    else:
        data[phase].insert(index, new_step)


def remove_step(data: CommentedMap, phase: str, index: int) -> bool:
    """
    Remove a step from a phase.

    Args:
        data: Test case YAML data
        phase: Phase name
        index: Step index to remove

    Returns:
        True if successful, False if step not found
    """
    if phase not in data:
        return False
    steps = data[phase]
    if not isinstance(steps, (list, CommentedSeq)) or index >= len(steps):
        return False

    del steps[index]
    return True
