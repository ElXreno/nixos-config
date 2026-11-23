token=$(clan secrets get tf-cloudflare-api-token)
if [ -z "$token" ]; then
  echo "clan secret tf-cloudflare-api-token is missing or empty" >&2
  exit 1
fi
export CLOUDFLARE_API_TOKEN=$token

CLOUDFLARE_ACCOUNT_ID=$(
  curl --silent --fail --get --config - \
    --data-urlencode "name=$CF_ZONE" \
    https://api.cloudflare.com/client/v4/zones <<<"header = \"Authorization: Bearer $token\"" |
    jq --exit-status --raw-output '.result[0].account.id'
)
export CLOUDFLARE_ACCOUNT_ID

cd "$(git rev-parse --show-toplevel)/cf-workers"

if [ $# -eq 0 ]; then
  set -- */
fi

for worker; do
  (cd "$worker" && wrangler deploy)
done
