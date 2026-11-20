# CloudSlash Kubernetes Provider Plugin

Official Kubernetes starter plugin for CloudSlash. Performs read-only discovery of pod resource requests/limits, persistent volumes (PVs/PVCs), orphaned LoadBalancer services, and node capacity.

## Capabilities
- **Provider**: `k8s`
- **Capabilities**: `scan` (read-only discovery; remediation contributions welcome)
- **Contract**: CloudSlash Plugin SDK v1.0.0

## Authentication
Supports standard Kubernetes client authentication:
- In-cluster ServiceAccount token (`/var/run/secrets/kubernetes.io/serviceaccount`)
- Local kubeconfig (`~/.kube/config` or `KUBECONFIG` env var)

## Standalone Build & Run
```bash
go build -o cloudslash-plugin-k8s .
./cloudslash-plugin-k8s
```

## Detaching / Extraction
This plugin is completely decoupled from CloudSlash core. To extract this plugin into its own standalone repository:
```bash
mkdir -p ../cloudslash-plugin-k8s
cp -r ./* ../cloudslash-plugin-k8s/
cd ../cloudslash-plugin-k8s
go mod init github.com/DrSkyle/cloudslash-plugin-k8s
go mod tidy
```
