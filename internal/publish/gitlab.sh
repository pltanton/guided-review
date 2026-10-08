#!/usr/bin/env bash
set -uo pipefail

dir=$(gr export --dir) || exit 1
x=$dir/review.json
host=$(jq -r .host "$x")
api=$(jq -r .api "$x")
log=$dir/published.jsonl
ok=1

drafts() { jq -r '.drafts[]?.file' "$x"; }

if [ -n "$(drafts)" ]; then
	if have=$(glab api --hostname "$host" --paginate "$api/draft_notes?per_page=100" | jq -s 'add // [] | map(.note)'); then
		ours=$(drafts | while read -r f; do jq .note "$dir/$f"; done | jq -s .)
		foreign=$(jq -n --argjson have "$have" --argjson ours "$ours" \
			'[$have[] | select(. as $n | $ours | index([$n]) | not)] | length')
		if [ "$foreign" -gt 0 ]; then
			echo "the MR holds $foreign draft notes that are not from this export:" \
				"publishing would post them too, ask the reviewer first" >&2
			ok=0
		fi
	else
		ok=0
	fi
	if [ $ok = 1 ]; then
		while read -r f; do
			if jq -e --argjson n "$(jq .note "$dir/$f")" 'index([$n])' <<<"$have" >/dev/null; then
				continue
			fi
			glab api --hostname "$host" -X POST "$api/draft_notes" \
				-H 'Content-Type: application/json' --input "$dir/$f" >/dev/null || { ok=0; break; }
		done < <(drafts)
	fi
	if [ $ok = 1 ]; then
		if glab api --hostname "$host" -X POST "$api/draft_notes/bulk_publish" >/dev/null; then
			jq -c '.drafts[] | del(.file)' "$x" >>"$log"
		else
			ok=0
		fi
	fi
fi

if [ $ok = 1 ] && [ "$(jq -r .approve "$x")" = true ]; then
	if glab api --hostname "$host" -X POST "$api/approve" >/dev/null; then
		echo '{"kind":"approve"}' >>"$log"
	else
		ok=0
	fi
fi

while [ $ok = 1 ] && read -r t; do
	id=$(jq -r .id <<<"$t")
	reply=$(jq -r '.reply // ""' <<<"$t")
	if [ -n "$reply" ]; then
		glab api --hostname "$host" -X POST "$(jq -r .reply_api <<<"$t")" -f body="$reply" >/dev/null || { ok=0; break; }
		jq -nc --arg id "$id" '{kind: "thread", id: $id, part: "reply"}' >>"$log"
	fi
	if [ "$(jq -r .resolve <<<"$t")" = true ]; then
		glab api --hostname "$host" -X PUT "$(jq -r .api <<<"$t")" -f resolved=true >/dev/null || { ok=0; break; }
		jq -nc --arg id "$id" '{kind: "thread", id: $id, part: "resolve"}' >>"$log"
	fi
done < <(jq -c '.threads[]?' "$x")

gr mark-published || ok=0
[ $ok = 1 ]
