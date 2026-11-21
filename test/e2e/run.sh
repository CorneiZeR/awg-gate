#!/usr/bin/env bash
# End-to-end test: issues a whole network with the image's own CLI, starts it in
# Docker and proves who reaches what - and who does not.
#
#   test/e2e/run.sh              # profile v1
#   PROFILE=v3 test/e2e/run.sh   # profile v3
#
# Environment: IMAGE (default awg-gate:e2e, built unless SKIP_BUILD=1),
# KEEP=1 leaves the bench running for a look around.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
PROFILE="${PROFILE:-v1}"
export IMAGE="${IMAGE:-awg-gate:e2e}"
export WORK="$here/work-$PROFILE"
compose=(docker compose -f "$here/compose.yaml" -p "awg-gate-e2e-$PROFILE")

failures=0
pass() { printf '  \033[32mok\033[0m    %s\n' "$1"; }
fail() { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; failures=$((failures + 1)); }
step() { printf '\n== %s\n' "$1"; }

cleanup() {
	if [[ "${KEEP:-}" == 1 ]]; then
		echo "KEEP=1: the bench is still running (${compose[*]} down -v to stop it)"
		return
	fi
	"${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ "${SKIP_BUILD:-}" != 1 ]]; then
	step "build $IMAGE"
	docker build -q -t "$IMAGE" "$root" >/dev/null
fi

step "issue a network (profile $PROFILE) with the image's own CLI"
rm -rf "$WORK"
mkdir -p "$WORK"/{config,data,a,b,clients}
cat >"$WORK/config/gate.yaml" <<EOF
# e2e bench
endpoint: hub:443
profile: $PROFILE
servers:
people:
EOF
cli() { docker run --rm --user "$(id -u):$(id -g)" -v "$WORK:/work" -w /work "$IMAGE" "$@"; }
cli hub-info --init --config config/gate.yaml --data data >"$WORK/hub.json"
yesterday="$(date -u -v-1d +%F 2>/dev/null || date -u -d yesterday +%F)"
cli issue server a --hub-info hub.json --gate config/gate.yaml --address 10.247.0.10 --ports 80,9090 --out a >/dev/null
cli issue server b --hub-info hub.json --gate config/gate.yaml --address 10.247.0.11 --out b >/dev/null
mv "$WORK/a/a.conf" "$WORK/a/awg.conf"
mv "$WORK/b/b.conf" "$WORK/b/awg.conf"
cli issue person alice laptop --hub-info hub.json --gate config/gate.yaml --address 10.247.1.10 --reach a --out clients >/dev/null
cli issue person bob laptop --hub-info hub.json --gate config/gate.yaml --address 10.247.1.20 --reach b --out clients >/dev/null
cli issue person carol laptop --hub-info hub.json --gate config/gate.yaml --address 10.247.1.30 --reach a --expires "$yesterday" --out clients >/dev/null
cli issue person dave laptop --hub-info hub.json --gate config/gate.yaml --reach a --out clients >/dev/null
# dave's client speaks plain WireGuard: the same keys without the obfuscation lines.
grep -v -E '^(Jc|Jmin|Jmax|S[1-4]|H[1-4]|HeaderProtectionKey) =' "$WORK/clients/dave-laptop.conf" >"$WORK/clients/dave-plain.conf"
cli check --config config/gate.yaml
# The CLI ran as this user and wrote 0600 files; the daemons run as root without
# CAP_DAC_OVERRIDE and cannot read another user's 0600 file. A real deployment
# installs them root-owned; the bench just lets everyone read its throwaway keys.
chmod -R a+rX "$WORK"

step "start the bench"
if ! out="$("${compose[@]}" up -d --quiet-pull 2>&1)"; then
	printf '%s\n' "$out" | tail -20
	exit 1
fi

on() { local who="$1"; shift; "${compose[@]}" exec -T "$who" "$@"; }
get() { on "$1" wget -qO- -T 3 "$2" 2>&1; }
eventually() { # eventually SECONDS DESCRIPTION COMMAND... - retries until the command succeeds
	local deadline=$((SECONDS + $1)) what="$2"
	shift 2
	until "$@" >/dev/null 2>&1; do
		if ((SECONDS >= deadline)); then fail "$what"; return 1; fi
		sleep 1
	done
	pass "$what"
}
never() { # never DESCRIPTION COMMAND... - the command must fail
	local what="$1"
	shift
	if "$@" >/dev/null 2>&1; then fail "$what"; else pass "$what"; fi
}
says() { get "$1" "$2" | grep -q "$3"; }

step "tunnels come up"
for svc in hub spoke-a spoke-b alice bob; do
	eventually 60 "$svc is healthy" on "$svc" awg-gate health
done
for svc in carol dave; do
	never "$svc gets no handshake" on "$svc" awg-gate health
done

step "people reach what they may"
eventually 10 "alice reaches server a through Traefik" says alice http://10.247.0.10/ server-a
eventually 10 "bob reaches server b through nginx" says bob http://10.247.0.11/ server-b
eventually 10 "alice pings the hub" on alice ping -c1 -W2 10.247.0.1

step "and nothing else"
never "alice cannot reach server b (not in her reach)" get alice http://10.247.0.11/
never "bob cannot reach server a (not in his reach)" get bob http://10.247.0.10/
never "alice cannot reach a:8080 (the hub allows a only 80 and 9090)" get alice http://10.247.0.10:8080/
never "alice cannot reach a:9090 (the spoke allows only 80 and 8080)" get alice http://10.247.0.10:9090/
never "alice cannot reach bob (people never reach people)" on alice ping -c1 -W2 10.247.1.20
never "server a cannot reach alice (servers never open connections to people)" on spoke-a ping -c1 -W2 10.247.1.10
never "server a cannot reach server b" on spoke-a ping -c1 -W2 10.247.0.11
never "expired carol reaches nothing" get carol http://10.247.0.10/
never "plain-WireGuard dave reaches nothing" get dave http://10.247.0.10/

step "from the internet"
# wget exits non-zero on a 403, so the answer is captured before it is looked at.
answer="$(get outsider http://198.18.0.10/ || true)"
if grep -q 403 <<<"$answer"; then
	pass "server a answers the internet 403 (Traefik ipAllowList)"
else
	fail "server a answers the internet 403 (Traefik ipAllowList)"
fi
never "server b does not answer the internet at all (lockdown)" get outsider http://198.18.0.11/
never "the hub answers nothing on the internet but the tunnel port" get outsider http://198.18.0.2/

step "status"
if on hub awg-gate status | grep -q 'alice/laptop'; then pass "hub status names alice/laptop"; else fail "hub status names alice/laptop"; fi

step "changes to gate.yaml apply live"
cp "$WORK/config/gate.yaml" "$WORK/gate.good"
echo "this: is: not: yaml" >"$WORK/config/gate.yaml.tmp" && mv "$WORK/config/gate.yaml.tmp" "$WORK/config/gate.yaml"
eventually 10 "a broken gate.yaml is reported" sh -c "${compose[*]} exec -T hub awg-gate status | grep -q REJECTED"
eventually 5 "and the hub keeps serving the last good one" says alice http://10.247.0.10/ server-a
cp "$WORK/gate.good" "$WORK/config/gate.yaml.tmp" && mv "$WORK/config/gate.yaml.tmp" "$WORK/config/gate.yaml"

cli revoke person bob --gate config/gate.yaml >/dev/null
"${compose[@]}" kill -s HUP hub >/dev/null
eventually 10 "revoked bob loses server b at once (SIGHUP)" sh -c "! ${compose[*]} exec -T bob wget -qO- -T 2 http://10.247.0.11/"

sed -i.bak "s/expires: \"\{0,1\}$yesterday\"\{0,1\}/expires: \"2099-12-31\"/" "$WORK/config/gate.yaml"
eventually 20 "carol, given a new end date, connects on the next re-read" says carol http://10.247.0.10/ server-a

step "result"
if ((failures > 0)); then
	echo "$failures check(s) failed (profile $PROFILE)"
	"${compose[@]}" logs --no-color hub spoke-a spoke-b | tail -60
	exit 1
fi
echo "all checks passed (profile $PROFILE)"
