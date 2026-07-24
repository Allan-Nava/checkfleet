# A single-node Patroni on top of the official postgres image, just enough to
# serve the Patroni REST API (:8008) that the `patroni` check module reads.
# This is integration-only scaffolding — never used by unit tests.
FROM postgres:16

RUN apt-get update \
 && apt-get install -y --no-install-recommends python3-pip python3-psycopg2 curl \
 && pip3 install --break-system-packages "patroni[etcd3]" \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/lib/postgresql/pgdata /var/run/postgresql \
 && chown -R postgres:postgres /var/lib/postgresql /var/run/postgresql

COPY patroni.yml /etc/patroni.yml

USER postgres
EXPOSE 8008 5432
HEALTHCHECK --interval=5s --timeout=3s --retries=30 \
  CMD curl -sf http://localhost:8008/health || exit 1
CMD ["patroni", "/etc/patroni.yml"]
