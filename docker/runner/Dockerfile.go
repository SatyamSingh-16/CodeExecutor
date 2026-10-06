FROM golang:1.22-bookworm

# Configure Go cache and temporary paths to point to writable /tmp
ENV GOCACHE=/tmp/.gocache \
    GOPATH=/tmp/go \
    GOTMPDIR=/tmp

# Install GNU time for peak memory and execution measurement; clean package caches
RUN apt-get update && \
    apt-get install -y --no-install-recommends time && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Create dedicated non-root sandbox user with UID/GID 1000
RUN groupadd -g 1000 sandbox && \
    useradd -u 1000 -g 1000 -d /tmp -s /bin/sh sandbox

# Install runner entrypoint
WORKDIR /runner
COPY entrypoint.sh /runner/entrypoint.sh
RUN chmod 755 /runner/entrypoint.sh

# Run as unprivileged sandbox user from /tmp
USER 1000:1000
WORKDIR /tmp

ENTRYPOINT ["/runner/entrypoint.sh"]
