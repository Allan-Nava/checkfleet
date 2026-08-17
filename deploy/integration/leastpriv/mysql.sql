-- Least-privilege user for the mysql check (CF-181). Exactly the grants in
-- docs/permissions.md — no SELECT on any schema.
CREATE USER 'checkfleet'@'%' IDENTIFIED BY 'checkfleet';
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'checkfleet'@'%';
