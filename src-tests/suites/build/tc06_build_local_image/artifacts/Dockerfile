# tsuite-mesh:local - Built from local source
# Contains MCP Mesh packages built from source code
#
# This image is built by src-tests after compiling all packages.
# It mirrors tsuite-mesh:X.Y.Z but uses locally built artifacts.

FROM python:3.11-slim

# Install system dependencies and Node.js 22
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    wget \
    git \
    build-essential \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Verify Node.js installation
RUN node --version && npm --version

# Create directories for local packages
RUN mkdir -p /wheels /packages /workspace

# Copy locally built artifacts
COPY wheels/*.whl /wheels/
COPY packages/*.tgz /packages/
COPY bin/meshctl /usr/local/bin/meshctl

# Make meshctl executable
RUN chmod +x /usr/local/bin/meshctl

# Install Python packages from local wheels
RUN pip install --no-cache-dir --find-links /wheels mcp-mesh-core mcp-mesh

# Verify mesh module installation
RUN python -c "import mesh; print('mesh module imported successfully')"

# Install meshctl CLI from local tarball (if exists) or use binary
RUN if ls /packages/mcpmesh-cli-*.tgz 1> /dev/null 2>&1; then \
        npm install -g /packages/mcpmesh-cli-*.tgz; \
    else \
        echo "Using meshctl binary from /usr/local/bin"; \
    fi

# Verify meshctl installation
RUN meshctl --version

# Install TypeScript SDK globally from local tarball
RUN npm install -g /packages/mcpmesh-sdk-*.tgz

# Install tsuite dependencies for running tests inside container
RUN pip install --no-cache-dir click rich pyyaml jsonpath-ng requests flask docker

# Create mcp-mesh user for security (matching official runtime images)
RUN groupadd -r mcp-mesh && useradd -r -g mcp-mesh mcp-mesh

# Set ownership
RUN chown -R mcp-mesh:mcp-mesh /workspace /wheels /packages

# Set working directory
WORKDIR /workspace

# Labels for identification
LABEL org.opencontainers.image.title="tsuite-mesh"
LABEL org.opencontainers.image.description="MCP Mesh Test Suite Base Image (Local Build)"
LABEL org.opencontainers.image.version="local"
LABEL mcp-mesh.build.type="local"

# Default command
CMD ["bash"]
