# CLM Deployment Scripts

Automated deployment scripts for HyperFleet CLM (Cluster Lifecycle Management) components.

## Overview

The `deploy-clm.sh` script automates the installation and uninstallation of HyperFleet CLM components (API, Sentinel, and Adapters) using Helm for E2E testing environments. It provides a consistent and repeatable deployment process with comprehensive validation and error handling.

## Features

- **Install/Uninstall Operations**: Deploy or remove all CLM components with a single command
- **Configurable Components**: Install all components or selectively skip specific ones
- **Image Customization**: Configure custom image repositories and tags for each component
- **Helm Chart Management**: Automatically clone and use Helm charts from component repositories
- **Pod Health Verification**: Automatic verification of pod health after deployment with failure detection (CrashLoopBackOff, ImagePullBackOff, etc.)
- **Namespace Lifecycle**: Automatic namespace creation and deletion
- **Infrastructure Validation**: Pre-deployment checks for cluster readiness
- **Dry-Run Support**: Test deployment without making changes
- **Verbose Logging**: Detailed logging for troubleshooting
- **Error Handling**: Comprehensive validation and graceful error handling with automatic log retrieval on failures

## Prerequisites

The script requires the following tools to be installed:

- `kubectl` - Kubernetes command-line tool
- `helm` - Helm package manager (v3+)
- `git` - Git version control

Ensure you have:
- Valid kubeconfig with access to target cluster
- Appropriate RBAC permissions for namespace and resource management
- Network access to component Git repositories and image registries

## Quick Start

### Option 1: Using Command-Line Flags (Simple)

Install all components with default settings:

```bash
./deploy-scripts/deploy-clm.sh --action install --namespace <unique_namespace>
```

Install with custom image tags:

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --api-image-tag v1.2.0 \
    --sentinel-image-tag v1.2.0 \
    --adapter-image-tag v1.2.0
```

Uninstall all components:

```bash
./deploy-scripts/deploy-clm.sh --action uninstall --namespace <unique_namespace>
```

### Option 2: Using .env File (Recommended for Complex Configurations)

For easier management of deployment parameters, use a `.env` file:

1. **Copy the example configuration:**
   ```bash
   cd deploy-scripts/
   cp .env.example .env
   ```

2. **Edit `.env` with your settings:**
   ```bash
   vim .env  # or your preferred editor
   ```

   Key parameters you can configure:
   - `NAMESPACE` - Kubernetes namespace (default: `hyperfleet-e2e-$USER`)
   - `IMAGE_REGISTRY` - Container image registry
   - `API_IMAGE_TAG`, `SENTINEL_IMAGE_TAG`, `ADAPTER_IMAGE_TAG` - Image tags
   - `GCP_PROJECT_ID` - Google Cloud Project ID for Pub/Sub
   - `INSTALL_API`, `INSTALL_SENTINEL`, `INSTALL_ADAPTER` - Component selection

   See [.env.example](.env.example) for all available parameters.

3. **Run the deployment:**
   ```bash
   ./deploy-clm.sh --action install
   ```

**Configuration Priority:**
- Command-line flags override .env file values
- .env file values override script defaults
- This allows baseline config in `.env` with per-run overrides via flags

## Command-Line Reference

For basic usage, see [Quick Start](#quick-start) section above.

### Basic Syntax

```bash
./deploy-scripts/deploy-clm.sh --action <install|uninstall> [OPTIONS]
```

### Required Flags

| Flag | Description |
|------|-------------|
| `--action <action>` | Action to perform: `install` or `uninstall` |

### Optional Flags

#### General Options

| Flag | Description | Default |
|------|-------------|---------|
| `--namespace <namespace>` | Kubernetes namespace for deployment | `hyperfleet-e2e-$USER` |
| `--dry-run` | Print commands without executing | `false` |
| `--verbose` | Enable verbose logging | `false` |
| `--help` | Show help message | - |

#### Component Selection

| Flag | Description |
|------|-------------|
| `--skip-api` | Skip API component installation |
| `--skip-sentinel` | Skip Sentinel component installation |
| `--skip-adapter` | Skip Adapter component installation |

#### Image Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `--image-registry <registry>` | Image registry for all components | `registry.ci.openshift.org/ci` |
| `--api-image-repo <repo>` | API image repository (without registry) | `hyperfleet-api` |
| `--api-image-tag <tag>` | API image tag | `latest` |
| `--sentinel-image-repo <repo>` | Sentinel image repository (without registry) | `hyperfleet-sentinel` |
| `--sentinel-image-tag <tag>` | Sentinel image tag | `latest` |
| `--adapter-image-repo <repo>` | Adapter image repository (without registry) | `hyperfleet-adapter` |
| `--adapter-image-tag <tag>` | Adapter image tag | `latest` |

**Notes**:
- Helm chart sources are fixed and pulled from the official component repositories at the `main` branch
- Final image path format: `${IMAGE_REGISTRY}/${IMAGE_REPO}:${IMAGE_TAG}`
- Example: `registry.ci.openshift.org/ci/hyperfleet-api:latest`

## Examples

### Installation Examples

#### 1. Install with Default Settings

```bash
./deploy-scripts/deploy-clm.sh --action install --namespace <unique_namespace>
```

This installs all three components (API, Sentinel, Adapter) with default configurations.

#### 2. Install Only API and Sentinel

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --skip-adapter
```

#### 3. Install with Custom Image Tags

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --api-image-tag v1.2.0 \
    --sentinel-image-tag v1.2.0 \
    --adapter-image-tag v1.2.0
```

#### 4. Install with Custom Image Repository

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --api-image-repo myregistry.io/hyperfleet-api \
    --api-image-tag pr-123
```

#### 5. Dry-Run Installation (No Changes)

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --dry-run \
    --verbose
```

This simulates the installation without making any actual changes.

### Uninstallation Examples

#### 1. Uninstall All Components

```bash
./deploy-scripts/deploy-clm.sh --action uninstall --namespace <unique_namespace>
```

This removes all Helm releases.

#### 2. Dry-Run Uninstallation

```bash
./deploy-scripts/deploy-clm.sh --action uninstall \
    --namespace <unique_namespace> \
    --dry-run \
    --verbose
```

#### 3. Uninstall Specific Components Only

```bash
./deploy-scripts/deploy-clm.sh --action uninstall \
    --namespace <unique_namespace> \
    --skip-api \
    --skip-sentinel
```

This only uninstalls the Adapter component.

## Script Workflow

### Installation Flow

1. **Dependency Checks**: Validates that `kubectl`, `helm`, and `git` are available
2. **Context Validation**: Verifies kubectl context and cluster connectivity
3. **Chart Cloning**: Clones Helm charts from Git repositories
4. **Component Installation**: Installs components in order (API → Sentinel → Adapter) using `helm upgrade --install` with `--create-namespace`
5. **Pod Health Verification**: Verifies all pods are running and healthy (detects CrashLoopBackOff, ImagePullBackOff, etc.)
6. **Status Reporting**: Displays deployment status and usage instructions

If any component fails health verification, the script automatically retrieves pod logs for troubleshooting and exits with an error status.

### Uninstallation Flow

1. **Dependency Checks**: Validates required tools
2. **Context Validation**: Verifies kubectl context
3. **User Confirmation**: Prompts for confirmation (unless `--dry-run`)
4. **Component Removal**: Uninstalls Helm releases in reverse order (Adapter → Sentinel → API) - this automatically removes all resources
5. **Cleanup**: Removes temporary working directories

## Namespace Management

The script leverages Helm's built-in namespace management:

- **Installation**: Namespace is automatically created by Helm using the `--create-namespace` flag
- **Uninstallation**: Resources are removed by `helm uninstall`, but the namespace is **not deleted**
- **Uniqueness**: Each deployment requires a unique namespace to prevent GCP Pub/Sub resource collisions.

If you want to completely remove the namespace after uninstallation:

```bash
# Uninstall components
./deploy-scripts/deploy-clm.sh --action uninstall --namespace <unique_namespace>

# Manually delete namespace if desired
kubectl delete namespace <unique_namespace>
```

This design allows you to:
- Reuse the same namespace for multiple install/uninstall cycles
- Keep other resources in the namespace that aren't managed by Helm
- Manually inspect resources after uninstallation for debugging

## Troubleshooting

### Debugging

Use `--dry-run --verbose` flags to see what the script would do without making changes:

```bash
./deploy-scripts/deploy-clm.sh --action install \
    --namespace <unique_namespace> \
    --dry-run \
    --verbose
```

Check Helm deployment status:

```bash
helm list -n <namespace>
kubectl get pods -n <namespace>
kubectl logs -n <namespace> <pod-name>
```

View script execution with bash trace:

```bash
bash -x deploy-scripts/deploy-clm.sh --action install --namespace <unique_namespace>
```

## Integration with E2E Tests

### Pre-Test Setup

Before running E2E tests, deploy the CLM components:

```bash
# Deploy test environment
./deploy-scripts/deploy-clm.sh --action install --namespace <unique_namespace>

# Configure E2E test API URL
EXTERNAL_IP=$(kubectl get svc hyperfleet-api -n $NAMESPACE -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export HYPERFLEET_API_URL="http://${EXTERNAL_IP}:8000"

# Run E2E tests
./bin/hyperfleet-e2e test --label-filter=tier0
```

### Post-Test Cleanup

After tests complete:

```bash
./deploy-scripts/deploy-clm.sh --action uninstall --namespace <unique_namespace>
```

## Script Output

The script provides structured log output with the following levels:

- **[INFO]**: Informational messages
- **[SUCCESS]**: Successful operations
- **[WARNING]**: Warnings (non-critical)
- **[ERROR]**: Errors (critical failures)
- **[VERBOSE]**: Detailed debug information (when `--verbose` is enabled)

## Best Practices

1. **Use Dry-Run First**: Always test with `--dry-run` before actual deployment
2. **Namespace Isolation**: Use dedicated namespaces for different test environments
3. **Tag Specificity**: Use specific image tags instead of `latest` for reproducible deployments
4. **Cleanup**: Always cleanup test environments after use to save resources
5. **Verbose Logging**: Use `--verbose` when troubleshooting issues
6. **Version Alignment**: Deploy matching versions of all components together

