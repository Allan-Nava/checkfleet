// Least-privilege user for the mongodb check (CF-181): clusterMonitor only,
// with no read on any application database.
db.getSiblingDB("admin").createUser({
  user: "checkfleet",
  pwd: "checkfleet",
  roles: [{ role: "clusterMonitor", db: "admin" }],
});
