# CloudSlash Linode provider

Read-only discovery of Linode instances, Block Storage volumes, and NodeBalancers through the Akamai Linode v4 API. The plugin streams canonical CRN nodes and preserves API errors as failed or partial scans. It does not implement quarantine, remediation, pricing, or automatic savings estimates.

Set `LINODE_TOKEN` in the plugin environment with `linodes:read_only`, `volumes:read_only`, and `nodebalancers:read_only` scopes. Set `LINODE_ACCOUNT_ID` to a stable, unique account namespace, or pass `account_id` in scan configuration. Use the same namespace on subsequent scans; never put a token in it. This namespace identifies collected inventory and is not proof of authorization for a cloud mutation.

Each request has a 30-second timeout and a 4 MiB response limit. A scan is limited to 15 minutes, 1,000 pages per resource family, and 100,000 resources overall. The plugin rejects duplicate identities during pagination rather than claiming a changing inventory is complete. Costs remain explicitly unknown.

Build with `go build -o cloudslash-plugin-Linode .` using the checked-in SDK replacement. Run through CloudSlash's signed-plugin loading workflow; the plugin binary speaks go-plugin gRPC and is not a standalone command-line scanner.

API references: [instances](https://techdocs.akamai.com/linode-api/reference/get-linode-instances), [NodeBalancers](https://techdocs.akamai.com/linode-api/reference/get-node-balancers), [pagination](https://techdocs.akamai.com/linode-api/reference/pagination), [scopes](https://techdocs.akamai.com/linode-api/reference/get-started).
