#!/usr/bin/env bash
#
# kind-local.sh — Manage a local kind cluster for E2E testing
#
# Wraps kind, helm, and deploy-clm.sh into a single workflow.
# Sources deploy-scripts/.env for all config (copy from .env.example).
#
# Usage:
#   ./deploy-scripts/kind-local.sh up                           # Full setup
#   ./deploy-scripts/kind-local.sh setup                        # Cluster + RabbitMQ + Maestro + images
#   ./deploy-scripts/kind-local.sh deploy                       # Deploy components
#   ./deploy-scripts/kind-local.sh port-forward                 # Forward API + Maestro
#   ./deploy-scripts/kind-local.sh rebuild [component]          # Rebuild image + restart
#   ./deploy-scripts/kind-local.sh rebuild --no-cache [comp]    # Force rebuild
#   ./deploy-scripts/kind-local.sh down                         # Tear down
#
# See docs/local-kind-setup.md for the full guide.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ============================================================================
# Configuration — sourced from .env, with local-only defaults below
# ============================================================================

# shellcheck source=.env
[[ -f "${SCRIPT_DIR}/.env" ]] && source "${SCRIPT_DIR}/.env"

# Local-only defaults (not in .env unless user added them)
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
KIND_CONTEXT="kind-${KIND_CLUSTER}"
INFRA_DIR="${INFRA_DIR:-${HOME}/projects/hyperfleet-infra}"
PROJECTS_DIR="${PROJECTS_DIR:-${HOME}/projects}"
MAESTRO_NS="${MAESTRO_NS:-maestro}"
MAESTRO_CONSUMER="${MAESTRO_CONSUMER:-cluster1}"
MAESTRO_LOCAL_PORT="${MAESTRO_LOCAL_PORT:-8100}"
RABBITMQ_URL="${RABBITMQ_URL:-amqp://guest:guest@rabbitmq:5672}"

# Override .env defaults for local kind
NAMESPACE="${NAMESPACE:-hyperfleet-local}"
IMAGE_PULL_POLICY="IfNotPresent"
API_SERVICE_TYPE="ClusterIP"

# Map .env adapter names
CLUSTER_ADAPTERS="${CLUSTER_TIER0_ADAPTERS_DEPLOYMENT:-cl-namespace,cl-job,cl-deployment,cl-maestro}"
NODEPOOL_ADAPTERS="${NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT:-np-configmap}"

# ============================================================================
# Helpers
# ============================================================================

require_kind_context() {
  if ! kubectl config get-contexts "${KIND_CONTEXT}" &>/dev/null; then
    echo "ERROR: kind context ${KIND_CONTEXT} not found. Run: kind create cluster --name ${KIND_CLUSTER}"
    exit 1
  fi
  local current
  current="$(kubectl config current-context 2>/dev/null || true)"
  if [[ "${current}" != "${KIND_CONTEXT}" ]]; then
    echo "Switching to kind context: ${KIND_CONTEXT}"
    kubectl config use-context "${KIND_CONTEXT}"
  fi
}

kill_port_forwards() {
  pkill -f "kubectl.*port-forward.*hyperfleet-api" 2>/dev/null || true
  pkill -f "kubectl.*port-forward.*maestro" 2>/dev/null || true
  sleep 1
}

# ============================================================================
# Commands
# ============================================================================

cmd_setup() {
  echo "=== Creating kind cluster ==="
  kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$" || kind create cluster --name "${KIND_CLUSTER}"
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl --context "${KIND_CONTEXT}" apply -f -

  echo "=== Installing RabbitMQ ==="
  kubectl --context "${KIND_CONTEXT}" apply -f "${INFRA_DIR}/manifests/rabbitmq.yaml" --namespace "${NAMESPACE}"
  echo "Waiting for RabbitMQ..."
  local retries=60
  until kubectl --context "${KIND_CONTEXT}" get pod -l app=rabbitmq -n "${NAMESPACE}" --no-headers 2>/dev/null | grep -q .; do
    ((retries--)) || { echo "ERROR: Timed out waiting for RabbitMQ pod"; exit 1; }
    sleep 2
  done
  kubectl --context "${KIND_CONTEXT}" wait --for=condition=ready pod -l app=rabbitmq --namespace "${NAMESPACE}" --timeout=120s

  echo "=== Installing Maestro ==="
  make -C "${INFRA_DIR}" install-maestro NAMESPACE="${MAESTRO_NS}" KUBECONFIG="${HOME}/.kube/config"
  make -C "${INFRA_DIR}" create-maestro-consumer MAESTRO_CONSUMER="${MAESTRO_CONSUMER}" NAMESPACE="${MAESTRO_NS}" KUBECONFIG="${HOME}/.kube/config"

  echo "=== Building images ==="
  "${SCRIPT_DIR}/kind-build-images.sh" "$@"
}

cmd_deploy() {
  require_kind_context

  echo "=== Deploying API + Sentinels + Adapters ==="
  SENTINEL_BROKER_RABBITMQ_URL="${RABBITMQ_URL}" \
  ADAPTER_BROKER_RABBITMQ_URL="${RABBITMQ_URL}" \
  ADAPTER_BROKER_TYPE=rabbitmq \
  SENTINEL_BROKER_TYPE=rabbitmq \
  IMAGE_PULL_POLICY="${IMAGE_PULL_POLICY}" \
  NAMESPACE="${NAMESPACE}" \
  API_SERVICE_TYPE="${API_SERVICE_TYPE}" \
  API_ADAPTERS_CLUSTER="${CLUSTER_ADAPTERS}" \
  API_ADAPTERS_NODEPOOL="${NODEPOOL_ADAPTERS}" \
  CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_ADAPTERS}" \
  NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_ADAPTERS}" \
  "${SCRIPT_DIR}/deploy-clm.sh" --action install
}

cmd_down() {
  require_kind_context

  kill_port_forwards

  NAMESPACE="${NAMESPACE}" \
  CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_ADAPTERS}" \
  NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_ADAPTERS}" \
  "${SCRIPT_DIR}/deploy-clm.sh" --action uninstall --delete-k8s-resources
}

cmd_port_forward() {
  kill_port_forwards

  kubectl --context "${KIND_CONTEXT}" port-forward -n "${NAMESPACE}" svc/hyperfleet-api 8000:8000 &
  kubectl --context "${KIND_CONTEXT}" port-forward -n "${MAESTRO_NS}" svc/maestro "${MAESTRO_LOCAL_PORT}":8000 &

  local api_ready=false
  for _ in $(seq 1 10); do
    sleep 2
    if curl -sf http://localhost:8000/api/hyperfleet/v1/clusters > /dev/null 2>&1; then
      api_ready=true
      break
    fi
  done
  if [[ "${api_ready}" == true ]]; then
    echo "API ready at http://localhost:8000"
  else
    echo "ERROR: API not reachable at localhost:8000"
    exit 1
  fi
  if curl -sf "http://localhost:${MAESTRO_LOCAL_PORT}/api/maestro/v1/consumers" > /dev/null 2>&1; then
    echo "Maestro ready at http://localhost:${MAESTRO_LOCAL_PORT}"
  else
    echo "WARNING: Maestro not reachable at localhost:${MAESTRO_LOCAL_PORT}"
  fi
}

# rebuild — Rebuild image(s), load into kind, restart affected deployments.
# Args forwarded to kind-build-images.sh (component names, --no-cache).
cmd_rebuild() {
  require_kind_context

  "${SCRIPT_DIR}/kind-build-images.sh" "$@"

  # Figure out what to restart based on args (skip --no-cache flag)
  local components=()
  for arg in "$@"; do
    [[ "${arg}" == --* ]] && continue
    components+=("${arg}")
  done

  if [[ ${#components[@]} -eq 0 ]]; then
    echo "=== Restarting all deployments (excluding postgresql) ==="
    local deploys
    deploys=$(kubectl --context "${KIND_CONTEXT}" get deployments -n "${NAMESPACE}" -o name \
      | grep -v postgresql)
    echo "${deploys}" | xargs kubectl --context "${KIND_CONTEXT}" rollout restart -n "${NAMESPACE}"
    echo "${deploys}" | xargs -I{} kubectl --context "${KIND_CONTEXT}" rollout status {} -n "${NAMESPACE}" --timeout=120s
  else
    for comp in "${components[@]}"; do
      echo "=== Restarting ${comp} ==="
      kubectl --context "${KIND_CONTEXT}" rollout restart deployment \
        -n "${NAMESPACE}" -l "app.kubernetes.io/name=${comp},app.kubernetes.io/component!=postgresql"
      kubectl --context "${KIND_CONTEXT}" rollout status deployment \
        -n "${NAMESPACE}" -l "app.kubernetes.io/name=${comp}" --timeout=120s
    done
  fi

  echo "=== Re-establishing port-forwards ==="
  cmd_port_forward
}

cmd_up() {
  cmd_setup "$@"
  cmd_deploy
  cmd_port_forward
}

# ============================================================================
# Entrypoint
# ============================================================================

case "${1:-}" in
  up)           shift; cmd_up "$@" ;;
  setup)        shift; cmd_setup "$@" ;;
  deploy)       cmd_deploy ;;
  down|undeploy) cmd_down ;;
  port-forward) cmd_port_forward ;;
  rebuild)      shift; cmd_rebuild "$@" ;;
  *)
    echo "Usage: $0 {up|setup|deploy|down|port-forward|rebuild}"
    echo ""
    echo "  up [COMPONENTS...]              Full setup from scratch"
    echo "  setup [COMPONENTS...]           Cluster + RabbitMQ + Maestro + build images"
    echo "  deploy                          Deploy API + sentinels + adapters"
    echo "  down                            Remove all + kill port-forwards"
    echo "  port-forward                    Forward API (:8000) + Maestro (:${MAESTRO_LOCAL_PORT})"
    echo "  rebuild [--no-cache] [COMP...]  Rebuild image(s) + restart + port-forward"
    echo ""
    echo "  COMPONENTS: e.g. 'hyperfleet-adapter' (default: all)"
    echo "  Config: deploy-scripts/.env (copy from .env.example)"
    exit 1
    ;;
esac
