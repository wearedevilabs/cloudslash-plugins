# CloudSlash Azure Provider Plugin

Official Microsoft Azure provider plugin for CloudSlash. Collects Azure Virtual Machines, Managed Disks, Azure SQL databases, and public IPs, executing safe optimizations.

## Capabilities
- **Provider**: `azure`
- **Capabilities**: `scan`, `quarantine`, `remediation`
- **Contract**: CloudSlash Plugin SDK v2

## Authentication
Supports default Azure credentials chain:
- `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`
- Azure Managed Identity (MSI)
- Azure CLI (`az login`)

## Standalone Build & Run
```bash
go build -o cloudslash-plugin-azure .
./cloudslash-plugin-azure
```

## Detaching / Extraction
This plugin is completely decoupled from CloudSlash core. To extract this plugin into its own standalone repository:
```bash
mkdir -p ../cloudslash-plugin-azure
cp -r ./* ../cloudslash-plugin-azure/
cd ../cloudslash-plugin-azure
go mod init github.com/DrSkyle/cloudslash-plugin-azure
go mod tidy
```
