#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
CLEANUP_SCRIPT="${SCRIPT_DIR}/cleanup-pubsub-resources.sh"
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/hyperfleet-pubsub-cleanup-test.XXXXXX")
trap 'rm -rf "${TEST_DIR}"' EXIT

assert_equal() {
    local expected="$1"
    local actual="$2"
    local message="$3"

    if [[ "$expected" != "$actual" ]]; then
        printf 'FAIL: %s\nexpected:\n%s\nactual:\n%s\n' "$message" "$expected" "$actual" >&2
        exit 1
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local message="$3"

    if [[ "$haystack" != *"$needle"* ]]; then
        printf 'FAIL: %s\nmissing: %s\noutput:\n%s\n' "$message" "$needle" "$haystack" >&2
        exit 1
    fi
}

assert_not_contains() {
    local haystack="$1"
    local needle="$2"
    local message="$3"

    if [[ "$haystack" == *"$needle"* ]]; then
        printf 'FAIL: %s\nunexpected: %s\noutput:\n%s\n' "$message" "$needle" "$haystack" >&2
        exit 1
    fi
}

mkdir -p "${TEST_DIR}/bin"

# gcloud stub. Lists read fixture files (full resource paths, as the real CLI
# prints them with --format="value(name)"). Deletes are recorded in order.
# Delete errors use the format of the real CLI (surface/pubsub/topics/delete.py).
#   GCLOUD_LIST_FAIL=subscriptions|topics  make that list call fail
#   GCLOUD_DELETE_NOTFOUND=<name>          delete of <name> reports a missing resource
#   GCLOUD_DELETE_FAIL=<name>              delete of <name> fails with GCLOUD_DELETE_MESSAGE
cat >"${TEST_DIR}/bin/gcloud" <<'EOF'
#!/usr/bin/env bash
# args: pubsub <subscriptions|topics> <list|delete> [name] ...
kind="$2"
action="$3"
if [[ "${action}" == "list" ]]; then
    if [[ "${GCLOUD_LIST_FAIL:-}" == "${kind}" ]]; then
        echo "ERROR: (gcloud.pubsub.${kind}.list) PERMISSION_DENIED" >&2
        exit 1
    fi
    cat "${FIXTURE_DIR:?}/${kind}.txt"
    exit 0
fi
name="$4"
if [[ "$5" != "--project=p" || "$6" != "--quiet" ]]; then
    echo "stub: unexpected delete flags: ${*:5}" >&2
    exit 3
fi
printf 'delete %s %s\n' "${kind}" "${name}" >> "${GCLOUD_CALLS_FILE:?}"
if [[ "${GCLOUD_DELETE_NOTFOUND:-}" == "${name}" ]]; then
    echo "ERROR: Failed to delete ${kind%s} [projects/p/${kind}/${name}]: Resource not found (resource=${name})." >&2
    echo "ERROR: (gcloud.pubsub.${kind}.delete) Failed to delete the following: [${name}]." >&2
    exit 1
fi
if [[ "${GCLOUD_DELETE_FAIL:-}" == "${name}" ]]; then
    echo "ERROR: (gcloud.pubsub.${kind}.delete) ${GCLOUD_DELETE_MESSAGE:-PERMISSION_DENIED}" >&2
    exit 1
fi
exit 0
EOF

chmod +x "${TEST_DIR}/bin/gcloud"

# new_case writes the fixtures for one case and resets the call log.
# usage: new_case <subscriptions> <topics>
new_case() {
    export FIXTURE_DIR="${TEST_DIR}/fixtures"
    export GCLOUD_CALLS_FILE="${TEST_DIR}/gcloud-calls"
    rm -rf "${FIXTURE_DIR}"
    mkdir -p "${FIXTURE_DIR}"
    printf '%s' "$1" >"${FIXTURE_DIR}/subscriptions.txt"
    printf '%s' "$2" >"${FIXTURE_DIR}/topics.txt"
    : >"${GCLOUD_CALLS_FILE}"
    unset GCLOUD_LIST_FAIL GCLOUD_DELETE_NOTFOUND GCLOUD_DELETE_FAIL GCLOUD_DELETE_MESSAGE
}

# run_script runs the script with the stub first on PATH and sets
# OUTPUT (stdout and stderr) and STATUS without tripping set -e.
run_script() {
    STATUS=0
    OUTPUT=$(PATH="${TEST_DIR}/bin:${PATH}" GCP_PROJECT_ID="" "${CLEANUP_SCRIPT}" "$@" 2>&1) || STATUS=$?
}

calls() {
    cat "${GCLOUD_CALLS_FILE}"
}

sub() { printf 'projects/p/subscriptions/%s\n' "$1"; }
topic() { printf 'projects/p/topics/%s\n' "$1"; }

# Only the named namespace is cleaned, subscriptions before topics.
new_case "$(sub e2e-111-clusters-a)"$'\n'"$(sub e2e-222-clusters-a)"$'\n' \
    "$(topic e2e-111-clusters)"$'\n'"$(topic e2e-111-clusters-a-dlq)"$'\n'"$(topic e2e-222-clusters)"$'\n'
run_script -n e2e-111 -p p
assert_equal "0" "${STATUS}" "cleanup succeeds"
assert_equal $'delete subscriptions e2e-111-clusters-a\ndelete topics e2e-111-clusters\ndelete topics e2e-111-clusters-a-dlq' \
    "$(calls)" "only the named namespace's resources are deleted, by bare name"
echo "PASS: cleanup deletes only the named namespace's resources, subscriptions first"

# Matching is by the "<namespace>-" prefix alone, so every entity kind is
# cleaned, including one the script has never heard of. Names that contain the
# namespace without starting with "<namespace>-" are kept.
new_case "$(sub e2e-111-parents-a)"$'\n'"$(sub e2e-1110-clusters-a)"$'\n' \
    "$(topic e2e-111-parents)"$'\n'"$(topic e2e-111-newkind)"$'\n'"$(topic e2e-111-wifconfigs-a-dlq)"$'\n'"$(topic e2e-111)"$'\n'"$(topic rc-e2e-111-clusters)"$'\n'"$(topic hyperfleet-e2e-111-clusters)"$'\n'
run_script -n e2e-111 -p p
assert_equal "0" "${STATUS}" "cleanup succeeds"
assert_equal $'delete subscriptions e2e-111-parents-a\ndelete topics e2e-111-parents\ndelete topics e2e-111-newkind\ndelete topics e2e-111-wifconfigs-a-dlq' \
    "$(calls)" "every kind under the namespace prefix is deleted, and nothing else"
echo "PASS: cleanup matches the namespace prefix, whatever the entity kind"

# Dry run lists candidates and deletes nothing.
new_case "$(sub e2e-111-clusters-a)"$'\n' "$(topic e2e-111-clusters)"$'\n'
run_script -n e2e-111 -p p --dry-run
assert_equal "0" "${STATUS}" "dry run succeeds"
assert_equal "" "$(calls)" "dry run issues no gcloud delete"
assert_contains "${OUTPUT}" "[DRY-RUN] Would delete topic: e2e-111-clusters" "dry run names the candidate"
echo "PASS: dry run deletes nothing"

# A failed list is not an empty list.
new_case "$(sub e2e-111-clusters-a)"$'\n' "$(topic e2e-111-clusters)"$'\n'
export GCLOUD_LIST_FAIL=topics
run_script -n e2e-111 -p p
assert_equal "1" "${STATUS}" "a failed topic list fails the cleanup"
assert_contains "${OUTPUT}" "Failed to list Pub/Sub topics" "the list failure is logged"
new_case "$(sub e2e-111-clusters-a)"$'\n' "$(topic e2e-111-clusters)"$'\n'
export GCLOUD_LIST_FAIL=subscriptions
run_script -n e2e-111 -p p
assert_equal "1" "${STATUS}" "a failed subscription list fails the cleanup"
assert_contains "${OUTPUT}" "Failed to list Pub/Sub subscriptions" "the list failure is logged"
echo "PASS: cleanup fails when a Pub/Sub list fails"

# A resource that is already gone is a success.
new_case "" "$(topic e2e-111-clusters)"$'\n'
export GCLOUD_DELETE_NOTFOUND=e2e-111-clusters
run_script -n e2e-111 -p p
assert_equal "0" "${STATUS}" "a missing resource on delete counts as success"
assert_contains "${OUTPUT}" "Topic already deleted: e2e-111-clusters" "the already-deleted topic is logged"
new_case "$(sub e2e-111-clusters-a)"$'\n' ""
export GCLOUD_DELETE_NOTFOUND=e2e-111-clusters-a
run_script -n e2e-111 -p p
assert_equal "0" "${STATUS}" "a missing subscription on delete counts as success"
assert_contains "${OUTPUT}" "Subscription already deleted: e2e-111-clusters-a" "the already-deleted subscription is logged"
echo "PASS: cleanup tolerates a resource that is already gone"

# Every other delete error fails, including ones that read like a missing resource.
new_case "$(sub e2e-111-clusters-a)"$'\n' "$(topic e2e-111-clusters)"$'\n'
export GCLOUD_DELETE_FAIL=e2e-111-clusters
run_script -n e2e-111 -p p
assert_equal "1" "${STATUS}" "another delete error fails the cleanup"
assert_contains "${OUTPUT}" "PERMISSION_DENIED" "the delete error text is logged"
assert_contains "$(calls)" "delete subscriptions e2e-111-clusters-a" "the subscription is deleted before the failure"
for message in "Your default credentials were not found." "The specified key file does not exist." "Project 'p' not found or deleted."; do
    new_case "" "$(topic e2e-111-clusters)"$'\n'
    export GCLOUD_DELETE_FAIL=e2e-111-clusters GCLOUD_DELETE_MESSAGE="${message}"
    run_script -n e2e-111 -p p
    assert_equal "1" "${STATUS}" "'${message}' is a failure, not an already deleted resource"
    assert_not_contains "${OUTPUT}" "already deleted" "'${message}' is not logged as already deleted"
done
echo "PASS: cleanup fails on every other delete error"

echo "All cleanup-pubsub-resources tests passed"
