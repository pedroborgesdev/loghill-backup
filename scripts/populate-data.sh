#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DATA_DIR="$ROOT_DIR/data"
SENDERS_DIR="$DATA_DIR/senders"
EXECUTIONS_FILE="$DATA_DIR/executions/executions.jsonl"
MONITORING_EXECUTIONS_FILE="$DATA_DIR/monitoring-executions.jsonl"

command -v jq >/dev/null || {
  echo "error: jq is required" >&2
  exit 1
}

for required_file in "$EXECUTIONS_FILE" "$MONITORING_EXECUTIONS_FILE"; do
  if [[ ! -f "$required_file" ]]; then
    echo "error: missing data file: $required_file" >&2
    exit 1
  fi
done

now=$(date -u +%s)
now_iso=$(date -u -d "@$now" +%Y-%m-%dT%H:%M:%SZ)
sender_index=0

for sender_dir in "$SENDERS_DIR"/*; do
  [[ -d "$sender_dir" ]] || continue

  sender_id=$(basename "$sender_dir")
  instance_log=$(find "$sender_dir/instances" -mindepth 2 -maxdepth 2 -type f -name logs.txt -print | sort | head -n 1)
  if [[ -z "$instance_log" ]]; then
    echo "error: no instance log found for sender: $sender_id" >&2
    exit 1
  fi

  instance_id=$(basename "$(dirname "$instance_log")")
  instance_name="${sender_id}-heartbeat"
  sender_now=$((now - 30 - sender_index * 45))
  sender_now_iso=$(date -u -d "@$sender_now" +%Y-%m-%dT%H:%M:%SZ)

  jq -cn \
    --arg timestamp "$sender_now_iso" \
    --arg sender "$sender_id" \
    --arg instance_id "$instance_id" \
    --arg instance "$instance_name" \
    '{timestamp:$timestamp,sender:$sender,instance_id:$instance_id,severity:"INFO",message:"Sender heartbeat received",metadata:{environment:"production",instance:$instance}}' \
    >> "$instance_log"

  tmp_file=$(mktemp)
  jq -c --argjson latest "$sender_now" \
    'to_entries as $entries | $entries[] as $item | $item.value | .timestamp=(($latest - (($entries|length - 1 - $item.key) * 300)) | todateiso8601)' \
    <(jq -s '.' "$instance_log") > "$tmp_file"
  mv "$tmp_file" "$instance_log"

  line_count=$(wc -l < "$instance_log")
  file_size=$(stat -c %s "$instance_log")
  tmp_file=$(mktemp)
  jq \
    --arg id "$instance_id" \
    --arg now "$sender_now_iso" \
    --argjson count "$line_count" \
    --argjson size "$file_size" \
    '(.items[] | select(.id == $id)) |= (.last_activity_at=$now | .last_healthcheck_at=$now | .status="online" | .log_line_count=$count | .log_file_size=$size)' \
    "$sender_dir/instances.json" > "$tmp_file"
  mv "$tmp_file" "$sender_dir/instances.json"

  total_count=0
  total_size=0
  while IFS= read -r log_file; do
    total_count=$((total_count + $(wc -l < "$log_file")))
    total_size=$((total_size + $(stat -c %s "$log_file")))
  done < <(find "$sender_dir" -type f -path '*/instances/*/logs.txt' -print)

  tmp_file=$(mktemp)
  jq \
    --arg now "$sender_now_iso" \
    --argjson count "$total_count" \
    --argjson size "$total_size" \
    '.status="online" | .updated_at=$now | .last_activity_at=$now | .last_healthcheck_at=$now | .inactive_at=null | .expires_at=null | .log_line_count=$count | .log_file_size=$size' \
    "$sender_dir/sender.json" > "$tmp_file"
  mv "$tmp_file" "$sender_dir/sender.json"
  sender_index=$((sender_index + 1))
done

repopulate_executions() {
  local source_file=$1
  local target_file
  target_file=$(mktemp)

  jq -c --argjson now "$now" \
    'to_entries | .[] | .value as $value | ($now - 300 - (.key * 1200)) as $start | ($start | todateiso8601) as $started | $value | .started_at=$started | (if .finished_at then (.duration_ms // 0) as $duration | .finished_at=(($start + (($duration / 1000) | floor)) | todateiso8601) else . end) | .updated_at=(if .finished_at then .finished_at else .started_at end)' \
    <(jq -s '.' "$source_file") > "$target_file"
  mv "$target_file" "$source_file"
}

repopulate_monitoring_executions() {
  local source_file=$1
  local target_file
  target_file=$(mktemp)

  jq -c --argjson now "$now" \
    'to_entries | .[] | .value as $value | ($now - 300 - (.key * 1200)) as $start | ($start | todateiso8601) as $started | $value | .started_at=$started | (if .finished_at then .finished_at=(($start + 1) | todateiso8601) else . end)' \
    <(jq -s '.' "$source_file") > "$target_file"
  mv "$target_file" "$source_file"
}

repopulate_executions "$EXECUTIONS_FILE"
repopulate_monitoring_executions "$MONITORING_EXECUTIONS_FILE"

echo "data populated at $now_iso"