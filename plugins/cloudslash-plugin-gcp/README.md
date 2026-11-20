# CloudSlash GCP Provider Plugin

Official Google Cloud Platform (GCP) starter plugin for CloudSlash. Performs read-only discovery of Compute Engine VM instances, persistent disks, Cloud SQL instances, and Cloud Storage buckets.

## Capabilities
- **Provider**: `gcp`
- **Capabilities**: `scan` (read-only discovery; remediation contributions welcome)
- **Contract**: CloudSlash Plugin SDK v1.0.0

## Authentication
Supports Google Application Default Credentials (ADC):
- `GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json`
- Workload Identity Federation
- GCloud SDK (`gcloud auth application-default login`)

## Standalone Build & Run
```bash
go build -o cloudslash-plugin-gcp .
./cloudslash-plugin-gcp
```

## Detaching / Extraction
This plugin is completely decoupled from CloudSlash core. To extract this plugin into its own standalone repository:
```bash
mkdir -p ../cloudslash-plugin-gcp
cp -r ./* ../cloudslash-plugin-gcp/
cd ../cloudslash-plugin-gcp
go mod init github.com/DrSkyle/cloudslash-plugin-gcp
go mod tidy
```
