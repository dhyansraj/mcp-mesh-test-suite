from setuptools import setup, find_packages
from pathlib import Path

# Read README for long description
readme_path = Path(__file__).parent / "README.md"
long_description = ""
if readme_path.exists():
    long_description = readme_path.read_text()

setup(
    name="mcp-mesh-tsuite",
    version="0.1.0",
    description="YAML-driven integration test framework with container isolation",
    long_description=long_description,
    long_description_content_type="text/markdown",
    author="MCP Mesh Team",
    url="https://github.com/dhyansraj/mcp-mesh-test-suite",
    license="MIT",
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Topic :: Software Development :: Testing",
        "Topic :: Software Development :: Quality Assurance",
    ],
    keywords="testing, integration-testing, yaml, docker, mcp-mesh",
    packages=find_packages(),
    include_package_data=True,
    package_data={
        "tsuite": [
            "dashboard/**/*",
            "dashboard/*",
            "man/content/*.md",
        ],
    },
    install_requires=[
        "click>=8.0.0",
        "pyyaml>=6.0",
        "jsonpath-ng>=1.5.0",
        "docker>=6.0.0",
        "flask>=2.0.0",
        "requests>=2.28.0",
        "rich>=13.0.0",
    ],
    entry_points={
        "console_scripts": [
            "mcp-mesh-tsuite=tsuite.cli:main",
            "tsuite=tsuite.cli:main",
        ],
    },
    python_requires=">=3.10",
)
