-- Least-privilege user for the postgres check (CF-181). These are exactly the
-- statements published in docs/permissions.md: if the check needs more than
-- this, the integration suite fails and the document is wrong.
CREATE ROLE checkfleet LOGIN PASSWORD 'checkfleet';
GRANT pg_monitor TO checkfleet;
GRANT CONNECT ON DATABASE postgres TO checkfleet;
