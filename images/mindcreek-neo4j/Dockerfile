ARG NEO4J_BASE=neo4j:2025.10.1
FROM ${NEO4J_BASE}
# Use the APOC Core distributed with this exact Neo4j image. No startup download.
USER root
RUN cp /var/lib/neo4j/labs/apoc-*-core.jar /var/lib/neo4j/plugins/apoc.jar \
    && chown neo4j:neo4j /var/lib/neo4j/plugins/apoc.jar
LABEL org.opencontainers.image.title="MindCreek Neo4j with offline APOC Core" \
      org.opencontainers.image.version="2025.10.1-graph1"
