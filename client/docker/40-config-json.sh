#!/bin/sh
# Pravi config.json sa adresom gateway-a pri pokretanju kontejnera (T-32.1),
# da adresa ne bi bila zakucana u image-u. nginx ga servira na /config.json.
set -eu

: "${GATEWAY_URL:?GATEWAY_URL mora biti postavljen (npr. http://localhost:8080)}"

case "$GATEWAY_URL" in
  http://*|https://*) ;;
  *) echo "GATEWAY_URL mora pocinjati sa http:// ili https://" >&2; exit 1 ;;
esac
case "$GATEWAY_URL" in
  *\"*|*\\*) echo "GATEWAY_URL sadrzi nedozvoljene znakove" >&2; exit 1 ;;
esac

mkdir -p /tmp/app-config
printf '{\n  "gatewayUrl": "%s"\n}\n' "$GATEWAY_URL" > /tmp/app-config/config.json
echo "config.json: gatewayUrl=$GATEWAY_URL"
