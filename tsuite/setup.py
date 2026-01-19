from setuptools import setup, find_packages

setup(
    name="tsuite",
    version="0.1.0",
    description="YAML-driven integration test framework with container isolation",
    packages=find_packages(),
    install_requires=[
        "click>=8.0.0",
        "pyyaml>=6.0",
        "jsonpath-ng>=1.5.0",
        "docker>=6.0.0",
        "flask>=2.0.0",
        "requests>=2.28.0",
        "rich>=13.0.0",  # For nice terminal output
    ],
    entry_points={
        "console_scripts": [
            "tsuite=tsuite.cli:main",
        ],
    },
    python_requires=">=3.10",
)
