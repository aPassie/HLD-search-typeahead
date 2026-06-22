#!/usr/bin/env bash
# Download one hour of English Wikipedia pageviews and transform it into a
# `query,count` CSV with >=100k rows.
#
#   ./scripts/fetch_dataset.sh [YYYYMMDD-HH0000] [out.csv]
#   ./scripts/fetch_dataset.sh 20240101-120000 data/queries.csv
#
# Note: the download is a few hundred MB. Requires curl, zcat, awk, sort.
set -euo pipefail

HOUR="${1:-20240101-120000}"          # YYYYMMDD-HH0000
OUT="${2:-data/queries.csv}"
YEAR="${HOUR:0:4}"; MONTH="${HOUR:4:2}"
URL="https://dumps.wikimedia.org/other/pageviews/${YEAR}/${YEAR}-${MONTH}/pageviews-${HOUR}.gz"

mkdir -p "$(dirname "$OUT")"
echo "downloading $URL ..."
curl -fSL "$URL" -o /tmp/pageviews.gz

echo "transforming -> $OUT ..."
# pageviews format: "domain title count bytes". Keep English main-namespace pages,
# turn underscores into spaces, lowercase, drop namespaced/odd titles, then
# aggregate duplicate titles by summing counts.
zcat /tmp/pageviews.gz \
  | awk '$1=="en" && $3 ~ /^[0-9]+$/ { t=$2; gsub(/_/," ",t); if (t ~ /:/) next; print tolower(t) "," $3 }' \
  | awk -F, 'length($1) >= 1 && $2+0 > 1' \
  | sort -t, -k1,1 \
  | awk -F, '{ if ($1==p) s+=$2; else { if (p!="") print p","s; p=$1; s=$2 } } END { if (p!="") print p","s }' \
  > "$OUT"

echo "wrote $(wc -l < "$OUT") rows to $OUT"
