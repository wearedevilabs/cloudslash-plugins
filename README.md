# CloudSlash Plugins

Open-source plugins and starter runtimes for [CloudSlash Core](https://github.com/wearedevilabs/CloudSlash-Core).

CloudSlash Core is strictly provider-agnostic. Plugins run out of process over gRPC (built on the Apache-2.0 `github.com/DrSkyle/cloudslash-plugin-sdk` v1.0.0) so you can connect any cloud, cluster, database, or internal platform without touching Core.

## Write a Plugin in 5 Minutes

### Any language (Python, Bash, or Node.js)

```bash
# Scaffold and install a script plugin into ~/.cloudslash/plugins/
./new-plugin.sh myplatform python

# Edit scan.py to print JSON:
# {"provider":"myplatform","account":"acct","resources":[{"id":"disk-1","type":"disk","service":"storage","region":"us-east-1","cost_monthly":12.5}]}

cloudslash scan --allow-unsigned
```

The script runtime (`runtime/script`) executes `scan.py`, `scan.sh`, `scan.js`, or `scan` next to the binary, forwards configuration via `CLOUDSLASH_<KEY>` environment variables, and streams the JSON resources to Core. See [`examples/stale-files`](examples/stale-files) for a working example.

### Compiled Go plugin (Discovery + Remediation)

```bash
cs plugin create my-platform
cd cloudslash-plugin-my-platform
cs plugin dev
cs scan --allow-unsigned
```

## Included Plugins

| Plugin | Status | Capabilities |
| :--- | :--- | :--- |
| [`plugins/cloudslash-plugin-aws`](plugins/cloudslash-plugin-aws) | Reference implementation | Discovery, least-privilege IAM manifest, remediation |
| [`plugins/cloudslash-plugin-azure`](plugins/cloudslash-plugin-azure) | Community starter | Read-only discovery |
| [`plugins/cloudslash-plugin-gcp`](plugins/cloudslash-plugin-gcp) | Community starter | Read-only discovery |
| [`plugins/cloudslash-plugin-k8s`](plugins/cloudslash-plugin-k8s) | Community starter | Read-only discovery |
| [`plugins/cloudslash-plugin-Linode`](plugins/cloudslash-plugin-Linode) | Community starter | Read-only discovery |
| [`plugins/cloudslash-plugin-postgres`](plugins/cloudslash-plugin-postgres) | Scaffold | Template ready to implement |

AWS is our full reference implementation showing both scanning and safe remediation. The other plugins are open community starters — PRs for remediation, richer resource coverage, or brand-new providers are welcome.

## License

Apache-2.0.
