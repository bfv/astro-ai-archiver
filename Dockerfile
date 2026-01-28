FROM alpine:latest

RUN mkdir -p /app/bin && \
    mkdir /app/config && \
    mkdir /app/data && \
    mkdir /app/db && \
    chown -R 1000:1000 /app/db

COPY bin/astro-ai-archiver-linux-amd64 /app/bin/astro-ai-archiver

WORKDIR /app

USER 1000

ENTRYPOINT ["/app/bin/astro-ai-archiver", "mcp-server", "--config", "/app/config/config.yaml"]
