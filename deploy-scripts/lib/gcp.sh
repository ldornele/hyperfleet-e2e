#!/usr/bin/env bash

# gcp.sh - Google Cloud Platform resource management functions
#
# This module handles discovery and cleanup of GCP resources (Pub/Sub topics and subscriptions)
# created during deployment.
#
# NAMESPACE requirements
# - Must be unique to prevent Pub/Sub topic/subscription collisions across deployments
# - Must be DNS-1123 compliant (lowercase alphanumeric, hyphens, start/end with alphanumeric)
# - Default: hyperfleet-e2e-$USER (when using .env configuration)

# ============================================================================
# Constants
# ============================================================================

# Resource types managed by the system
readonly RESOURCE_TYPES=("clusters" "nodepools")

# ============================================================================
# GCP Dependency Checking
# ============================================================================

check_gcp_dependencies() {
    log_verbose "Checking GCP CLI dependencies"

    if ! command -v gcloud &> /dev/null; then
        log_error "gcloud CLI not found"
        log_error "Please install Google Cloud SDK: https://cloud.google.com/sdk/docs/install"
        return 1
    fi

    local gcloud_version
    gcloud_version=$(gcloud --version 2>/dev/null | head -n1 || echo "unknown")
    log_verbose "Found gcloud: ${gcloud_version}"

    return 0
}

# ============================================================================
# GCP Pub/Sub Discovery Functions
# ============================================================================

discover_pubsub_topics() {
    local namespace="$1"
    local project_id="${GCP_PROJECT_ID}"

    log_verbose "Discovering Pub/Sub topics for namespace: ${namespace}"

    if [[ -z "${project_id}" ]]; then
        log_error "GCP_PROJECT_ID is not set"
        return 1
    fi

    # List topics that match the namespace pattern
    # NAMESPACE must be unique and DNS-1123 compliant (default: hyperfleet-e2e-$USER when using .env)
    # Topics are named:
    #   - ${NAMESPACE}-${resource_type}  (e.g., hyperfleet-e2e-jdoe-clusters, hyperfleet-e2e-jdoe-nodepools)
    #   - ${NAMESPACE}-${resource_type}-dlq  (e.g., hyperfleet-e2e-jdoe-clusters-dlq)
    #   - ${NAMESPACE}-${resource_type}-${adapter_name}-dlq  (e.g., hyperfleet-e2e-jdoe-clusters-adapter1-dlq)
    local topics=()
    local all_topics

    if ! all_topics=$(gcloud pubsub topics list --project="${project_id}" --format="value(name)" 2>/dev/null); then
        log_error "Failed to list Pub/Sub topics in project ${project_id}"
        log_error "Make sure you have authenticated with: gcloud auth login"
        return 1
    fi

    while IFS= read -r topic; do
        if [[ -z "${topic}" ]]; then
            continue
        fi

        # Extract topic name from full path (projects/{project}/topics/{topic-name})
        local topic_name="${topic##*/}"

        # Match topics with all naming patterns:
        # 1. Main topics: ${namespace}-${resource_type}
        # 2. DLQ topics (intended): ${namespace}-${resource_type}-dlq
        # 3. DLQ topics (temporary/Helm bug): ${namespace}-${resource_type}-${adapter_name}-dlq
        local matched=false
        for resource_type in "${RESOURCE_TYPES[@]}"; do
            if [[ "${topic_name}" == "${namespace}-${resource_type}" ]] || \
               [[ "${topic_name}" == "${namespace}-${resource_type}-dlq" ]] || \
               [[ "${topic_name}" =~ ^${namespace}-${resource_type}-.+-dlq$ ]]; then
                matched=true
                break
            fi
        done

        if [[ "${matched}" == "true" ]]; then
            topics+=("${topic_name}")
        fi
    done <<< "${all_topics}"

    if [[ ${#topics[@]} -eq 0 ]]; then
        log_verbose "No Pub/Sub topics found for namespace: ${namespace}" >&2
        return 1
    fi

    log_info "Found ${#topics[@]} Pub/Sub topic(s) for namespace ${namespace}:" >&2
    for topic in "${topics[@]}"; do
        log_info "  - ${topic}" >&2
    done

    # Export for use in other functions (stdout only)
    printf '%s\n' "${topics[@]}"
}

discover_pubsub_subscriptions() {
    local namespace="$1"
    local project_id="${GCP_PROJECT_ID}"

    log_verbose "Discovering Pub/Sub subscriptions for namespace: ${namespace}"

    if [[ -z "${project_id}" ]]; then
        log_error "GCP_PROJECT_ID is not set"
        return 1
    fi

    # List subscriptions that match the namespace pattern
    # NAMESPACE must be unique and DNS-1123 compliant (default: hyperfleet-e2e-$USER when using .env)
    # Subscriptions are named: ${NAMESPACE}-${resource_type}-${adapter_name}
    # Example: hyperfleet-e2e-jdoe-clusters-adapter1, <unique-namespace>-clusters-adapter1
    local subscriptions=()
    local all_subscriptions

    if ! all_subscriptions=$(gcloud pubsub subscriptions list --project="${project_id}" --format="value(name)" 2>/dev/null); then
        log_error "Failed to list Pub/Sub subscriptions in project ${project_id}"
        log_error "Make sure you have authenticated with: gcloud auth login"
        return 1
    fi

    while IFS= read -r subscription; do
        if [[ -z "${subscription}" ]]; then
            continue
        fi

        # Extract subscription name from full path (projects/{project}/subscriptions/{subscription-name})
        local subscription_name="${subscription##*/}"

        # Match subscriptions with the expected naming pattern:
        # ${namespace}-${resource_type}-${adapter_name}
        local matched=false
        for resource_type in "${RESOURCE_TYPES[@]}"; do
            if [[ "${subscription_name}" =~ ^${namespace}-${resource_type}-.+ ]]; then
                matched=true
                break
            fi
        done

        if [[ "${matched}" == "true" ]]; then
            subscriptions+=("${subscription_name}")
        fi
    done <<< "${all_subscriptions}"

    if [[ ${#subscriptions[@]} -eq 0 ]]; then
        log_verbose "No Pub/Sub subscriptions found for namespace: ${namespace}" >&2
        return 1
    fi

    log_info "Found ${#subscriptions[@]} Pub/Sub subscription(s) for namespace ${namespace}:" >&2
    for subscription in "${subscriptions[@]}"; do
        log_info "  - ${subscription}" >&2
    done

    # Export for use in other functions (stdout only)
    printf '%s\n' "${subscriptions[@]}"
}

# ============================================================================
# GCP Pub/Sub Deletion Functions
# ============================================================================

delete_pubsub_subscription() {
    local subscription_name="$1"
    local project_id="${GCP_PROJECT_ID}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would delete subscription: ${subscription_name}"
        return 0
    fi

    log_info "Deleting subscription: ${subscription_name}"

    if gcloud pubsub subscriptions delete "${subscription_name}" \
        --project="${project_id}" \
        --quiet 2>/dev/null; then
        log_success "Deleted subscription: ${subscription_name}"
        return 0
    else
        log_error "Failed to delete subscription: ${subscription_name}"
        return 1
    fi
}

delete_pubsub_topic() {
    local topic_name="$1"
    local project_id="${GCP_PROJECT_ID}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would delete topic: ${topic_name}"
        return 0
    fi

    log_info "Deleting topic: ${topic_name}"

    if gcloud pubsub topics delete "${topic_name}" \
        --project="${project_id}" \
        --quiet 2>/dev/null; then
        log_success "Deleted topic: ${topic_name}"
        return 0
    else
        log_error "Failed to delete topic: ${topic_name}"
        return 1
    fi
}

delete_all_pubsub_subscriptions() {
    local namespace="$1"

    log_section "Deleting Pub/Sub Subscriptions"

    # Discover subscriptions (stdout only contains resource names, stderr has logs)
    local subscriptions
    if ! subscriptions=$(discover_pubsub_subscriptions "${namespace}"); then
        log_info "No Pub/Sub subscriptions to delete"
        return 0
    fi

    # Delete each subscription
    local failed=0
    while IFS= read -r subscription; do
        if [[ -n "${subscription}" ]]; then
            if ! delete_pubsub_subscription "${subscription}"; then
                log_error "Failed to delete subscription: ${subscription}"
                ((failed++))
            fi
        fi
    done <<< "${subscriptions}"

    if [[ ${failed} -gt 0 ]]; then
        log_error "${failed} subscription(s) failed to delete"
        return 1
    else
        log_success "All subscriptions deleted successfully"
        return 0
    fi
}

delete_all_pubsub_topics() {
    local namespace="$1"

    log_section "Deleting Pub/Sub Topics"

    # Discover topics (stdout only contains resource names, stderr has logs)
    local topics
    if ! topics=$(discover_pubsub_topics "${namespace}"); then
        log_info "No Pub/Sub topics to delete"
        return 0
    fi

    # Delete each topic
    local failed=0
    while IFS= read -r topic; do
        if [[ -n "${topic}" ]]; then
            if ! delete_pubsub_topic "${topic}"; then
                log_error "Failed to delete topic: ${topic}"
                ((failed++))
            fi
        fi
    done <<< "${topics}"

    if [[ ${failed} -gt 0 ]]; then
        log_error "${failed} topic(s) failed to delete"
        return 1
    else
        log_success "All topics deleted successfully"
        return 0
    fi
}

# ============================================================================
# Main GCP Cleanup Function
# ============================================================================

cleanup_gcp_resources() {
    local namespace="$1"

    log_section "Cleaning Up GCP Resources"

    # Check GCP CLI dependencies
    if ! check_gcp_dependencies; then
        log_error "GCP CLI dependencies not available"
        return 1
    fi

    local cleanup_errors=0

    # Delete subscriptions first (subscriptions depend on topics)
    if ! delete_all_pubsub_subscriptions "${namespace}"; then
        log_warning "Some subscriptions failed to delete"
        ((cleanup_errors++))
    fi

    # Delete topics
    if ! delete_all_pubsub_topics "${namespace}"; then
        log_warning "Some topics failed to delete"
        ((cleanup_errors++))
    fi

    if [[ ${cleanup_errors} -gt 0 ]]; then
        log_warning "GCP resource cleanup completed with errors"
        return 1
    else
        log_success "GCP resource cleanup complete"
        return 0
    fi
}
