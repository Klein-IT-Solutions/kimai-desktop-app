#!/usr/bin/env bash
# Seeds a Kimai test instance with customers, projects and activities via the API.
# Usage: KIMAI_URL=http://localhost:8001 KIMAI_TOKEN=... scripts/kimai-seed.sh
set -euo pipefail
: "${KIMAI_URL:?set KIMAI_URL}" "${KIMAI_TOKEN:?set KIMAI_TOKEN}"

api() { # method path json
  curl -fsS --noproxy '*' -X "$1" "$KIMAI_URL$2" \
    -H "Authorization: Bearer $KIMAI_TOKEN" -H "Content-Type: application/json" \
    ${3:+-d "$3"}
}
id() { python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'; }

customer() { api POST /api/customers "{\"name\":\"$1\",\"country\":\"DE\",\"currency\":\"EUR\",\"timezone\":\"Europe/Berlin\",\"visible\":true}" | id; }
project()  { api POST /api/projects "{\"name\":\"$2\",\"customer\":$1,\"visible\":true,\"globalActivities\":true}" | id; }
activity() { api POST /api/activities "{\"name\":\"$2\"${1:+,\"project\":$1},\"visible\":true}" | id; }

acme=$(customer "ACME GmbH")
muster=$(customer "Muster & Söhne KG")
intern=$(customer "Intern")

web=$(project "$acme" "Website Relaunch")
app=$(project "$acme" "Mobile App")
shop=$(project "$muster" "Onlineshop")
orga=$(project "$intern" "Organisation")

activity "" "Entwicklung" >/dev/null
activity "" "Meeting" >/dev/null
activity "" "Projektmanagement" >/dev/null
activity "$web" "Design" >/dev/null
activity "$shop" "Support" >/dev/null
activity "$orga" "Weiterbildung" >/dev/null

echo "Seeded customers ($acme, $muster, $intern) and projects ($web, $app, $shop, $orga)."
