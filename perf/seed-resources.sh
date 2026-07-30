#!/bin/bash
# Seed the database with generic resources (channels, wifconfigs, versions) for
# realistic perf baselines. Generalizes seed-clusters.sh for the non-reconciling
# generic-resource kinds (Channel, WifConfig, Version) added on top of the shared
# resources/resource_labels/resource_conditions tables.
#
# Versions are nested under a channel (POST /channels/{parent_id}/versions), so
# seeding versions creates/reuses a single dedicated parent channel
# ("perf-seed-versions-parent") and seeds all versions under it. This gives the
# worst-case single-list-query shape (many versions under one channel) rather than
# spreading versions thinly across many channels.
#
# Usage:
#   ./perf/seed-resources.sh channels              # seed 1000 channels (default)
#   ./perf/seed-resources.sh wifconfigs 100         # seed 100 wifconfigs
#   ./perf/seed-resources.sh versions 500           # seed 500 versions under one parent channel
#   ./perf/seed-resources.sh channels status        # show channel counts in the database
#   ./perf/seed-resources.sh channels cleanup       # delete seeded channels only
#   ./perf/seed-resources.sh channels reset         # delete ALL channels (clean slate)
#   ./perf/seed-resources.sh versions reset         # delete all seeded versions AND the parent channel

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
[[ -f "$REPO_DIR/env/env.local" ]] && set -a && source "$REPO_DIR/env/env.local" && set +a

API_URL="${HYPERFLEET_API_URL:?ERROR: HYPERFLEET_API_URL is not set}"
API_BASE="$API_URL/api/hyperfleet/v1"
CURL_OPTS="--connect-timeout 10 --max-time 30"
SEED_LABEL="perf-seed"
PARENT_CHANNEL_NAME="perf-seed-versions-parent"

KIND="${1:-}"
ACTION="${2:-1000}"

case "$KIND" in
  channels)
    ENDPOINT="channels"
    PAYLOAD_FILE="$REPO_DIR/testdata/payloads/channels/channel-request.json"
    NAME_PREFIX="perf-seed-channel"
    ;;
  wifconfigs)
    ENDPOINT="wifconfigs"
    PAYLOAD_FILE="$REPO_DIR/testdata/payloads/wifconfigs/wifconfig-request.json"
    NAME_PREFIX="perf-seed-wifconfig"
    ;;
  versions)
    PAYLOAD_FILE="$REPO_DIR/testdata/payloads/versions/version-request.json"
    NAME_PREFIX="perf-seed-version"
    # ENDPOINT is resolved lazily below, once the parent channel ID is known.
    ;;
  *)
    echo "ERROR: Invalid or missing kind '$KIND'. Expected 'channels', 'wifconfigs', or 'versions'."
    echo ""
    echo "Usage:"
    echo "  $0 <channels|wifconfigs|versions> [count]     # seed N resources (default 1000)"
    echo "  $0 <channels|wifconfigs|versions> status       # show counts"
    echo "  $0 <channels|wifconfigs|versions> cleanup      # delete seeded resources only"
    echo "  $0 <channels|wifconfigs|versions> reset        # delete ALL (clean slate)"
    exit 1
    ;;
esac

# --- Functions ----------------------------------------------------------------

# Find (or create) the single dedicated parent channel used to seed versions.
# Idempotent: repeated runs reuse the same parent by name.
ensure_parent_channel() {
  local existing
  existing=$(curl -G -s $CURL_OPTS "$API_BASE/channels" \
    --data-urlencode "search=name='$PARENT_CHANNEL_NAME'" \
    --data-urlencode "size=1" \
    --http1.1 -H "Accept: application/json" | jq -r '.items[0].id // empty')

  if [[ -n "$existing" ]]; then
    echo "$existing"
    return 0
  fi

  local payload
  payload=$(jq --arg name "$PARENT_CHANNEL_NAME" --arg lbl "$SEED_LABEL" \
    '.name = $name | .labels[$lbl] = "true"' \
    "$REPO_DIR/testdata/payloads/channels/channel-request.json")

  local id
  id=$(curl -s $CURL_OPTS -X POST "$API_BASE/channels" \
    --http1.1 \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d "$payload" | jq -r '.id // empty')

  if [[ -z "$id" ]]; then
    echo "ERROR: failed to create parent channel '$PARENT_CHANNEL_NAME' for version seeding" >&2
    exit 1
  fi
  echo "$id"
}

# Create a single resource with a unique name via POST $API_BASE/$ENDPOINT.
create_resource() {
  local i=$1
  local name="${NAME_PREFIX}-$(printf '%04d' "$i")-$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' ')"
  local payload
  payload=$(jq --arg name "$name" --arg lbl "$SEED_LABEL" \
    '.name = $name | .labels[$lbl] = "true"' \
    "$PAYLOAD_FILE")

  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" $CURL_OPTS \
    -X POST "$API_BASE/$ENDPOINT" \
    --http1.1 \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d "$payload")

  if [[ "$status" == "201" ]]; then
    return 0
  else
    echo "WARN: $KIND $i returned HTTP $status" >&2
    return 1
  fi
}

# Fetch and delete resources in batches from $API_BASE/$ENDPOINT.
# Usage: delete_in_batches                          (all resources at $ENDPOINT)
#        delete_in_batches --data-urlencode "search=name like '$NAME_PREFIX-%'"
delete_in_batches() {
  local deleted=0
  local prev_batch=0
  local stale_rounds=0
  local max_stale=3
  declare -A seen

  while true; do
    local items http_code
    items=$(curl -G -s -w '\n%{http_code}' $CURL_OPTS "$API_BASE/$ENDPOINT" \
      --data-urlencode "size=1000" \
      "$@" \
      --http1.1 -H "Accept: application/json")
    http_code=$(echo "$items" | tail -1)
    items=$(echo "$items" | sed '$d')
    if [[ ! "$http_code" =~ ^2 ]]; then
      echo "  WARN: fetch returned HTTP $http_code, aborting cleanup early" >&2
      break
    fi

    local ids
    ids=$(echo "$items" | jq -r '.items[]?.id // empty')
    local batch
    batch=$(echo "$ids" | grep -c . || true)

    if [[ "$batch" -eq 0 ]]; then
      break
    fi

    if [[ "$batch" -eq "$prev_batch" ]]; then
      stale_rounds=$((stale_rounds + 1))
      if [[ "$stale_rounds" -ge "$max_stale" ]]; then
        echo "  WARN: $batch $KIND remain after $max_stale rounds with no progress, stopping"
        break
      fi
    else
      stale_rounds=0
    fi
    prev_batch=$batch

    while IFS= read -r id; do
      [[ -z "$id" ]] && continue
      [[ "$id" =~ ^[a-zA-Z0-9_-]+$ ]] || continue
      [[ -n "${seen[$id]:-}" ]] && continue
      seen[$id]=1
      local http_code
      http_code=$(curl -s -o /dev/null -w '%{http_code}' $CURL_OPTS -X DELETE "$API_BASE/$ENDPOINT/$id" --http1.1)
      if [[ "$http_code" =~ ^2 ]]; then
        deleted=$((deleted + 1))
      else
        echo "  WARN: DELETE $id returned HTTP $http_code"
      fi
      if (( deleted % 50 == 0 )); then
        echo "  Deleted $deleted"
      fi
    done <<< "$ids"
  done

  if [[ "$deleted" -eq 0 ]]; then
    echo "No $KIND found"
  else
    echo "Deleted $deleted $KIND"
  fi
}

cleanup_seeded() {
  echo "=== Cleaning up seeded $KIND ==="
  delete_in_batches --data-urlencode "search=name like '$NAME_PREFIX-%'"
}

status_resources() {
  local active
  active=$(curl -s $CURL_OPTS "$API_BASE/$ENDPOINT?size=1" --http1.1 -H "Accept: application/json" | jq '.total // 0')

  local seeded
  seeded=$(curl -G -s $CURL_OPTS "$API_BASE/$ENDPOINT" \
    --data-urlencode "search=name like '$NAME_PREFIX-%'" \
    --data-urlencode "size=1" \
    --http1.1 -H "Accept: application/json" | jq '.total // 0')

  echo "=== Database status ($KIND) ==="
  if [[ "$KIND" == "versions" ]]; then
    echo "Versions under parent '$PARENT_CHANNEL_NAME': $active"
  else
    echo "Active $KIND: $active"
  fi
  echo "  Seeded ($NAME_PREFIX-*): $seeded"
  echo "  Other: $(( active - seeded ))"
}

cleanup_all() {
  echo "=== Cleaning up ALL $KIND ==="
  delete_in_batches
  if [[ "$KIND" == "versions" ]]; then
    echo "Deleting parent channel '$PARENT_CHANNEL_NAME'"
    local http_code
    http_code=$(curl -s -o /dev/null -w '%{http_code}' $CURL_OPTS -X DELETE "$API_BASE/channels/$PARENT_ID" --http1.1)
    if [[ ! "$http_code" =~ ^2 ]]; then
      echo "  WARN: DELETE parent channel $PARENT_ID returned HTTP $http_code"
    fi
  fi
}

# --- Resolve ENDPOINT for versions (needs the parent channel first) -----------

if [[ "$KIND" == "versions" ]]; then
  PARENT_ID=$(ensure_parent_channel)
  if [[ ! "$PARENT_ID" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo "ERROR: parent channel ID '$PARENT_ID' failed validation" >&2
    exit 1
  fi
  ENDPOINT="channels/$PARENT_ID/versions"
  echo "Using parent channel '$PARENT_CHANNEL_NAME' (id: $PARENT_ID)"
fi

# --- Subcommand dispatch ------------------------------------------------------

if [[ "$ACTION" == "status" ]]; then
  status_resources
  exit 0
fi

if [[ "$ACTION" == "cleanup" ]]; then
  cleanup_seeded
  exit 0
fi

if [[ "$ACTION" == "reset" ]]; then
  if [[ "$KIND" == "channels" ]]; then
    parent_check=$(curl -G -s $CURL_OPTS "$API_BASE/channels" \
      --data-urlencode "search=name='$PARENT_CHANNEL_NAME'" \
      --data-urlencode "size=1" \
      --http1.1 -H "Accept: application/json" | jq -r '.items[0].id // empty')
    if [[ -n "$parent_check" ]]; then
      version_count=$(curl -s $CURL_OPTS "$API_BASE/channels/$parent_check/versions?size=1" \
        --http1.1 -H "Accept: application/json" | jq '.total // 0')
      if [[ "$version_count" -gt 0 ]]; then
        echo "ERROR: $version_count versions still exist under parent channel '$PARENT_CHANNEL_NAME'."
        echo "Channel deletion is blocked while versions exist (on_parent_delete: restrict)."
        echo "Run './perf/seed-resources.sh versions reset' first, then retry."
        exit 1
      fi
    fi
  fi

  total=$(curl -s $CURL_OPTS "$API_BASE/$ENDPOINT?size=1" --http1.1 -H "Accept: application/json" | jq '.total // 0')
  echo "WARNING: This will delete ALL $total $KIND at $API_URL"
  echo "kubectl context: $(kubectl config current-context 2>/dev/null || echo 'unknown')"
  read -r -p "Are you sure? (y/N) " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    cleanup_all
  else
    echo "Aborted."
  fi
  exit 0
fi

if ! [[ "$ACTION" =~ ^[0-9]+$ ]]; then
  echo "ERROR: Invalid argument '$ACTION'. Expected a number, 'status', 'cleanup', or 'reset'."
  exit 1
fi

COUNT="$ACTION"

# --- Seed resources (default) -------------------------------------------------

existing=$(curl -G -s $CURL_OPTS "$API_BASE/$ENDPOINT" \
  --data-urlencode "search=name like '$NAME_PREFIX-%'" \
  --data-urlencode "size=1" \
  --http1.1 -H "Accept: application/json" | jq '.total // 0')

if [[ "$existing" -ge "$COUNT" ]]; then
  echo "Already have $existing seeded $KIND (target: $COUNT). Nothing to do."
  exit 0
fi

to_create=$((COUNT - existing))
echo "=== Seeding $to_create $KIND (existing: $existing, target: $COUNT) ==="
echo "API: $API_URL"
echo ""

created=0
failed=0
for i in $(seq 1 "$to_create"); do
  if create_resource "$i"; then
    created=$((created + 1))
  else
    failed=$((failed + 1))
  fi
  if (( i % 50 == 0 )); then
    echo "  Progress: $i / $to_create (created: $created, failed: $failed)"
  fi
done

echo ""
echo "=== Seeding complete ==="
echo "Created: $created"
echo "Failed:  $failed"
echo ""
echo "To clean up: ./perf/seed-resources.sh $KIND cleanup"
