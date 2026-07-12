#!/usr/bin/env bash
set -euo pipefail

subnet="${1:-}"
if [[ -z "$subnet" ]]; then
  route_line="$(ip route show default | head -n1)"
  dev="$(awk '{for (i=1;i<=NF;i++) if ($i=="dev") print $(i+1)}' <<<"$route_line")"
  cidr="$(ip -o -f inet addr show dev "$dev" | awk '{print $4}' | head -n1)"
  ip="${cidr%/*}"
  subnet="$(awk -F. '{printf "%s.%s.%s.0/24", $1, $2, $3}' <<<"$ip")"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

nmap -Pn -p 22,3080,9001 --open "$subnet" -oX "$tmp/scan.xml" >/dev/null

hosts=()
while IFS= read -r ip; do
  hosts+=("$ip")
done < <(grep -oE 'addr="[0-9.]+"' "$tmp/scan.xml" | cut -d'"' -f2 | sort -u)

printf '{\n'
printf '  "subnet": %s,\n' "$(jq -Rn --arg v "$subnet" '$v')"
printf '  "hosts": [\n'

first=1
for ip in "${hosts[@]}"; do
  [[ "$first" -eq 0 ]] && printf ',\n'
  first=0

  ssh_open=false
  port_3080=false
  port_9001=false
  timeout 1 bash -c "</dev/tcp/$ip/22" >/dev/null 2>&1 && ssh_open=true || true
  timeout 1 bash -c "</dev/tcp/$ip/3080" >/dev/null 2>&1 && port_3080=true || true
  timeout 1 bash -c "</dev/tcp/$ip/9001" >/dev/null 2>&1 && port_9001=true || true

  keys="$(ssh-keyscan -T 3 -t ed25519,rsa,ecdsa "$ip" 2>/dev/null || true)"
  identity="unknown"
  if grep -q 'AAAAC3NzaC1lZDI1NTE5AAAAIBg+v7UlTDPKr6xr3z3rWzcqqmOvpDhsR8azUwuNqnd8' <<<"$keys"; then
    identity="known-current-10.0.0.100"
  elif grep -q 'AAAAC3NzaC1lZDI1NTE5AAAAIImGiLYuTDj8NDgM6UpsU5C8zKNe0xuYZ3DQA1+VIrFI' <<<"$keys"; then
    identity="known-former-10.0.0.12"
  fi

  jq -n \
    --arg ip "$ip" \
    --arg identity "$identity" \
    --argjson ssh "$ssh_open" \
    --argjson p3080 "$port_3080" \
    --argjson p9001 "$port_9001" \
    '{ip: $ip, identity: $identity, ssh: $ssh, port_3080: $p3080, port_9001: $p9001}' | sed 's/^/    /'
done

printf '\n  ]\n}\n'
