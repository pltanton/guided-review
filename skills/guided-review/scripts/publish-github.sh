#!/usr/bin/env bash
set -uo pipefail

dir=$(gr export --dir) || exit 1
x=$dir/review.json
host=$(jq -r .host "$x")
log=$dir/published.jsonl
ok=1

req=$(jq -r '.request // ""' "$x")
if [ -n "$req" ]; then
	if gh api --hostname "$host" -X POST "$(jq -r .api "$x")" --input "$dir/$req" >/dev/null; then
		jq -c '.review[]' "$x" >>"$log"
	else
		ok=0
	fi
fi

while [ $ok = 1 ] && read -r t; do
	id=$(jq -r .id <<<"$t")
	reply=$(jq -r '.reply // ""' <<<"$t")
	if [ -n "$reply" ]; then
		gh api --hostname "$host" -X POST "$(jq -r .reply_api <<<"$t")" -f body="$reply" >/dev/null || { ok=0; break; }
		jq -nc --arg id "$id" '{kind: "thread", id: $id, part: "reply"}' >>"$log"
	fi
	if [ "$(jq -r .resolve <<<"$t")" = true ]; then
		gh api --hostname "$host" graphql -f id="$id" \
			-f query='mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{isResolved}}}' \
			>/dev/null || { ok=0; break; }
		jq -nc --arg id "$id" '{kind: "thread", id: $id, part: "resolve"}' >>"$log"
	fi
done < <(jq -c '.threads[]?' "$x")

gr mark-published || ok=0
[ $ok = 1 ]
