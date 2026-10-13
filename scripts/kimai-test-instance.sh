#!/usr/bin/env bash
# Starts a local Kimai test instance in Docker, creates an API token and seeds
# test data. Prints the URL and token to use in the app or for integration tests.
#
#   scripts/kimai-test-instance.sh up     start + provision (idempotent)
#   scripts/kimai-test-instance.sh down   stop and delete all data
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f dev/kimai/docker-compose.yml)
URL=http://localhost:8001
TOKEN=kt_dev_token_0123456789abcdef0123456789
sql() { "${COMPOSE[@]}" exec -T db mariadb -ukimai -pkimai kimai -N -e "$1"; }

case "${1:-up}" in
down)
  "${COMPOSE[@]}" down -v
  exit 0
  ;;
up) ;;
*)
  echo "usage: $0 up|down" >&2
  exit 2
  ;;
esac

"${COMPOSE[@]}" up -d
echo -n "Waiting for Kimai "
for _ in $(seq 1 90); do
  if curl -fs --noproxy '*' -o /dev/null "$URL/en/login"; then break; fi
  echo -n "."
  sleep 2
done
echo
curl -fs --noproxy '*' -o /dev/null "$URL/en/login" || { echo "Kimai did not come up – see: ${COMPOSE[*]} logs kimai" >&2; exit 1; }

# Kimai has no CLI for API tokens; tokens are stored as plain values.
if [ "$(sql "SELECT COUNT(*) FROM kimai2_access_token WHERE token='$TOKEN'")" = 0 ]; then
  sql "INSERT INTO kimai2_access_token (user_id, token, name) SELECT id, '$TOKEN', 'Kimai Tracker (dev)' FROM kimai2_users WHERE username='admin'"
  # A non-UTC time zone makes the integration tests verify time-zone handling.
  sql "UPDATE kimai2_user_preferences p JOIN kimai2_users u ON u.id=p.user_id SET p.value='Europe/Berlin' WHERE u.username='admin' AND p.name='timezone'"
  sql "UPDATE kimai2_users SET alias='Kimai Admin' WHERE username='admin'"
fi

if [ "$(sql "SELECT COUNT(*) FROM kimai2_customers")" = 0 ]; then
  KIMAI_URL=$URL KIMAI_TOKEN=$TOKEN scripts/kimai-seed.sh
fi

cat <<INFO

Kimai test instance is ready.
  Web UI:     $URL  (admin / changeme123)
  API token:  $TOKEN

Integration tests:
  KIMAI_TEST_URL=$URL KIMAI_TEST_TOKEN=$TOKEN make test-integration
INFO
