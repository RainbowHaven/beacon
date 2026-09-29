import { defineRailway, project, service } from "railway/iac";

// This repository manages only the beacon service. Postgres and other
// resources stay as they are in each Railway environment.
// See https://docs.railway.com/infrastructure-as-code#multi-repo-projects
export const partial = "beacon";

export default defineRailway((ctx) => {
  const staging = ctx.environment === "staging";

  const beacon = service("beacon", {
    start: "/app/beacon server",
    healthcheck: "/healthz",
    healthcheckTimeout: 30,
    // Restart policy (ON_FAILURE, 5 retries) has no IaC field. Leave it on
    // the service in the Railway dashboard.
    // Staging wipes the schema before start. Production must not.
    ...(staging ? { preDeploy: "/app/beacon wipe-schema" } : {}),
  });

  return project("beacon", {
    resources: [beacon],
  });
});
