#!/usr/bin/env bash
set -euo pipefail

stage_dir="${1:?stage directory required}"
archive_partial="${2:?partial archive path required}"
archive_final="${3:?final archive path required}"
workflow="${4:?workflow required}"
revision="${5:?commit required}"
run_id="${6:?run ID required}"
command_status="${7:?command status required}"
started_at="${8:?start time required}"
finished_at="${9:?finish time required}"
duration_ms="${10:?duration required}"
events="$stage_dir/.events.jsonl"

: >"$events"

while IFS= read -r file; do
  jq -c '
    select(.Test != null and (.Action == "pass" or .Action == "fail" or .Action == "skip")) |
    {
      test_id: ("go/" + (.Package | sub("^github.com/0xveya/tethux/?"; "") | gsub("[^A-Za-z0-9]+"; "/") | ascii_downcase) + "/" + (.Test | ascii_downcase | gsub("[^a-z0-9]+"; "-"))),
      name: .Test,
      suite: "go",
      status: (if .Action == "pass" then "passed" elif .Action == "fail" then "failed" else "skipped" end),
      timing: {started_at:null,finished_at:(.Time // null),duration_ms: (((.Elapsed // 0) * 1000) | round)},
      attempt: 1,
      source: {file: null, symbol: .Test, line: null},
      features: [],
      parameters: {package: .Package},
      metrics: {},
      message: null,
      failure: (if .Action == "fail" then {kind:"assertion",phase:"test",expected:null,actual:null,error_code:"GO_TEST_FAILED",stack_trace:null} else null end),
      artifacts: [($artifact)],
      labels: {language:"go"}
    }' --arg artifact "${file#"$stage_dir/"}" "$file" >>"$events"
done < <(find "$stage_dir/artifacts" -type f -name 'go-test.jsonl' -print)

while IFS= read -r file; do
  jq -c '
    select(.schema == "tethux.provider-test/v1" and .operation != "summary") |
    {
      test_id: ("provider/" + .provider + "/" + (.api // "provider") + "/" + .operation + (if .image then "/" + (.image | split("/")[-1] | split(":")[0]) else "" end)),
      name: (.provider + " " + .operation),
      suite: "provider",
      status: (if .status == "passed" then "passed" else "failed" end),
      timing: {started_at:(.started_at // null),finished_at:(.finished_at // .timestamp // null),duration_ms: (.duration_ms // 0)},
      attempt: 1,
      source: {file:"cmd/virt/test.go",symbol:null,line:null},
      features: ["provider." + .provider, "provider.lifecycle." + .operation],
      parameters: {provider:.provider,image:(.image // null),api:(.api // null),host:(.host // null)},
      metrics: (.details // {}),
      message: (.error // null),
      failure: (if .status == "passed" then null else {kind:"process_exit",phase:.operation,expected:"operation passed",actual:(.error // "operation failed"),error_code:"PROVIDER_OPERATION_FAILED",stack_trace:null} end),
      artifacts: [($artifact)],
      labels: {level:"integration"}
    }' --arg artifact "${file#"$stage_dir/"}" "$file" >>"$events"
done < <(find "$stage_dir/artifacts" -type f -name 'providers.jsonl' -print)

while IFS= read -r file; do
  jq -c '
    select(.schema == "tethux.laptop-integration/v1") |
    {
      test_id: ("topology/" + .runtime + "/end-to-end"),
      name: (.runtime + " container UDP topology"),
      suite: "topology",
      status: (if .status == "passed" then "passed" else "failed" end),
      timing: {started_at:(.started_at // null),finished_at:(.finished_at // null),duration_ms:(.duration_ms // 0)}, attempt: 1,
      source: {file:"nix/scripts/topology-smoke.sh",symbol:null,line:null},
      features: ["topology.container", "tunnel.udp", "provider." + .runtime],
      parameters: {runtime:.runtime,host:.host,image:(.image // null),small_n:(.topology.small_n // null),large_n:(.topology.large_n // null)}, metrics: {duration_ms:(.duration_ms // 0)}, message: null, failure: null,
      artifacts: [($artifact)], labels: {level:"integration"}
    }' --arg artifact "${file#"$stage_dir/"}" "$file" >>"$events"
done < <(find "$stage_dir/artifacts" -type f -name 'summary.jsonl' -print)

while IFS= read -r file; do
  jq -c '
    select(.schema == "tethux.cross-host-link/v1" and .status == "passed") |
    {
      test_id: ("cross-host/managed-link/" + .provider),
      name: ("Cross-host managed link via " + .provider), suite:"cross-host",
      status:"passed", timing:{started_at:(.started_at // null),finished_at:(.finished_at // null),duration_ms:(.duration_ms // 0)}, attempt:1,
      source:{file:"cmd/virt/link.go",symbol:null,line:null},
      features:["topology.cross-host","tunnel.udp","provider." + .provider],
      parameters:{host:.host,address:.address,peer:.peer,provider:.provider},metrics:{},message:null,failure:null,
      artifacts:[($artifact)],labels:{level:"integration"}
    }' --arg artifact "${file#"$stage_dir/"}" "$file" >>"$events"
done < <(find "$stage_dir/artifacts" -type f -name 'cross-link.jsonl' -print)

if [[ "$command_status" -ne 0 ]]; then
  jq -nc --arg workflow "$workflow" --argjson code "$command_status" '
    {test_id:("workflow/"+$workflow+"/execution"),name:("Workflow "+$workflow),suite:"infrastructure",status:"error",timing:{duration_ms:0},attempt:1,source:null,features:[],parameters:{},metrics:{exit_code:$code},message:"workflow command exited unsuccessfully",failure:{kind:"process_exit",phase:"runner",expected:"exit code 0",actual:("exit code "+($code|tostring)),error_code:"WORKFLOW_COMMAND_FAILED",stack_trace:null},artifacts:["logs/runner.log"],labels:{level:"infrastructure"}}' \
    >>"$events"
fi

jq -s --arg run_id "$run_id" '{schema_version:1,run_id:$run_id,tests:.}' "$events" >"$stage_dir/results.json"
rm -f "$events"

total="$(jq '.tests | length' "$stage_dir/results.json")"
passed="$(jq '[.tests[] | select(.status == "passed")] | length' "$stage_dir/results.json")"
failed="$(jq '[.tests[] | select(.status == "failed")] | length' "$stage_dir/results.json")"
skipped="$(jq '[.tests[] | select(.status == "skipped")] | length' "$stage_dir/results.json")"
errored="$(jq '[.tests[] | select(.status == "error")] | length' "$stage_dir/results.json")"
overall=passed
if [[ "$errored" -gt 0 ]]; then overall=error; elif [[ "$failed" -gt 0 ]]; then overall=failed; fi

runner_file="$(find "$stage_dir/artifacts" -type f -name runner.json -print -quit)"
if [[ -n "$runner_file" ]]; then
  runner="$(jq -c . "$runner_file")"
else
  os_version="$(. /etc/os-release 2>/dev/null && printf '%s' "${PRETTY_NAME:-Linux}")"
  architecture="$(uname -m)"
  case "$architecture" in x86_64) architecture=amd64 ;; aarch64) architecture=arm64 ;; esac
  runner="$(jq -nc \
    --arg device "${TETHUX_DEVICE_ID:-$(hostname)}" --arg hostname "$(hostname)" \
    --arg os_version "$os_version" --arg kernel "$(uname -r)" --arg arch "$architecture" \
    --arg cpu "$(lscpu | awk -F: '/Model name/{sub(/^[[:space:]]+/,"",$2); print $2; exit}')" \
    --argjson memory "$(awk '/MemTotal/{print $2*1024}' /proc/meminfo | cut -d. -f1)" \
    '{device_id:$device,display_name:$device,hostname:$hostname,os:"linux",os_version:$os_version,kernel:$kernel,architecture:$arch,cpu:$cpu,memory_bytes:$memory}')"
fi

files_json="$stage_dir/.files.jsonl"
: >"$files_json"
while IFS= read -r file; do
  relative="${file#"$stage_dir/"}"
  case "$relative" in manifest.json|.files.jsonl) continue ;; esac
  media=text/plain
  type=artifact
  public=true
  case "$relative" in
    logs/*) type=log ;;
    configs/*.json) type=config; media=application/json ;;
    configs/*.yaml|configs/*.yml) type=config; media=application/yaml ;;
    configs/*) type=config ;;
    results.json) type=results; media=application/json ;;
    *.json) media=application/json ;;
    *.jsonl) media=application/x-ndjson ;;
    *.yaml|*.yml) media=application/yaml ;;
    *.pcap|*.pcapng) type=packet_capture; media=application/vnd.tcpdump.pcap; public=false ;;
  esac
  jq -nc --arg path "$relative" --arg type "$type" --arg media "$media" \
    --arg sha "$(sha256sum "$file" | awk '{print $1}')" \
    --argjson size "$(stat -c %s "$file")" --argjson public "$public" \
    '{path:$path,type:$type,media_type:$media,size_bytes:$size,sha256:$sha,public:$public}' >>"$files_json"
done < <(find "$stage_dir" -type f -print | sort)

source_type=local
source_provider=null
if [[ -n "${CI:-}" ]]; then source_type=ci; source_provider=woodpecker; fi
git_dirty=false
if ! git diff --quiet || ! git diff --cached --quiet; then git_dirty=true; fi

jq -n \
  --arg run_id "$run_id" --arg workflow "$workflow" --arg commit "$revision" \
  --arg branch "${CI_COMMIT_BRANCH:-$(git branch --show-current)}" --arg tag "${CI_COMMIT_TAG:-}" \
  --arg commit_time "$(date -u -d "$(git show -s --format=%cI "$revision" 2>/dev/null || date -u +%FT%TZ)" +%Y-%m-%dT%H:%M:%S.%3NZ)" \
  --arg started "$started_at" --arg finished "$finished_at" --argjson duration "$duration_ms" \
  --arg source_type "$source_type" --arg source_provider "$source_provider" \
  --arg trigger "${CI_PIPELINE_EVENT:-manual}" --argjson attempt "${CI_PIPELINE_ATTEMPT:-1}" \
  --arg overall "$overall" --argjson total "$total" --argjson passed "$passed" \
  --argjson failed "$failed" --argjson skipped "$skipped" --argjson errored "$errored" \
  --argjson runner "$runner" --argjson files "$(jq -s . "$files_json")" --argjson dirty "$git_dirty" \
  --arg go_version "$(go version 2>/dev/null | awk '{print $3}' || true)" \
  --arg runtime "${TETHUX_CONTAINER_RUNTIME:-}" \
  '{schema_version:1,run_id:$run_id,project:{id:"tethux",name:"tethux",repository:"github.com/0xveya/tethux"},source:{type:$source_type,provider:($source_provider|if .=="null" then null else . end),workflow:$workflow,job:$workflow,attempt:$attempt,trigger:$trigger},git:{commit_sha:$commit,branch:$branch,tag:($tag|if .=="" then null else . end),dirty:$dirty,commit_timestamp:$commit_time},timing:{started_at:$started,finished_at:$finished,duration_ms:$duration},runner:$runner,software:{go_version:$go_version,test_runner_version:"1",project_binary_version:"dev",ubridge_replacement_version:($commit[0:8]),container_runtime:($runtime|if .=="" then null else . end)},environment:{network_backend:"native",virtualization:"kvm",privileged:true,ipv6_enabled:true},summary:{status:$overall,total:$total,passed:$passed,failed:$failed,skipped:$skipped,errored:$errored},files:$files,labels:{test_level:"integration",network_mode:"bridge",runner_group:"physical-laptops"}}' \
  >"$stage_dir/manifest.json"
rm -f "$files_json"

jq -e --arg run_id "$run_id" '.schema_version == 1 and .run_id == $run_id and (.tests | type == "array") and ([.tests[].test_id] | unique | length) == (.tests | length) and all(.tests[]; (.test_id | test("^[a-z0-9][a-z0-9./-]*$")) and (.status | IN("passed","failed","skipped","error","cancelled")))' "$stage_dir/results.json" >/dev/null
jq -e --arg run_id "$run_id" --argjson total "$total" --argjson passed "$passed" --argjson failed "$failed" --argjson skipped "$skipped" --argjson errored "$errored" '.schema_version == 1 and .run_id == $run_id and .summary.total == $total and .summary.passed == $passed and .summary.failed == $failed and .summary.skipped == $skipped and .summary.errored == $errored and (.files | type == "array")' "$stage_dir/manifest.json" >/dev/null
while IFS= read -r path; do
  [[ "$path" != /* && "$path" != *".."* ]]
  file="$stage_dir/$path"
  [[ -f "$file" ]]
  expected="$(jq -r --arg path "$path" '.files[] | select(.path == $path) | .sha256' "$stage_dir/manifest.json")"
  [[ "$(sha256sum "$file" | awk '{print $1}')" == "$expected" ]]
done < <(jq -r '.files[].path' "$stage_dir/manifest.json")

tar --zstd -cf "$archive_partial" -C "$stage_dir" manifest.json results.json logs configs artifacts
mv "$archive_partial" "$archive_final"
rm -rf "$stage_dir"
printf 'test archive: %s\n' "$archive_final"
