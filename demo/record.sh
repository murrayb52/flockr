#!/bin/bash
# record.sh — record the README demo in a nested i3 (needs Xephyr, ffmpeg, xdotool).
# Uses the fake agent, so it needs no API keys and shows no personal details.
#   demo/record.sh [output.gif]
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-$repo/docs/demo.gif}
export FLOCKR_SIZE=${FLOCKR_SIZE:-1100x660} FLOCKR_WORK=${FLOCKR_WORK:-/tmp/flockr-nested}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

"$repo/demo/nested.sh" stop
rm -rf "$FLOCKR_WORK/data/flockr"
"$repo/demo/nested.sh" start
eval "$("$repo/demo/nested.sh" env)"
sleep 3

k() { xdotool key --delay 90 "$@"; }
ty() { xdotool type --delay 12 "$1"; }
term() { k super+Return; sleep 1.2; ty "$1"; k Return; sleep 0.6; }
mkdir -p /tmp/flockr-demo/bookshelf/{api,app} /tmp/flockr-demo/website

# workspace 2 has an agent of its own (watch its logo on the bar)
k super+2; sleep 0.5
term "cd /tmp/flockr-demo/website && clear && fakeagent claude --delay 9 'rewrite the pricing page copy'"
k super+1; sleep 0.5
term "cd /tmp/flockr-demo/bookshelf/api && clear && fakeagent claude --ask --delay 4 'add pagination to the reading-list endpoint'"
term "cd /tmp/flockr-demo/bookshelf/app && clear && fakeagent pi --delay 3 'add offline caching to the book detail screen'"
term "cd /tmp/flockr-demo/bookshelf && clear"

ffmpeg -y -loglevel error -f x11grab -draw_mouse 0 -framerate 12 -video_size "$FLOCKR_SIZE" -i "$DISPLAY" \
  -c:v libx264 -preset ultrafast -crf 16 "$tmp/demo.mp4" &
rec=$!
sleep 11          # agents work; Pi finishes, Claude asks for permission
k super+u         # jump to the agent that needs me
sleep 1.8; k y    # approve
sleep 4
k alt+slash; sleep 0.8; ty "reading-list API"; sleep 0.4; k Return   # label it
sleep 1.5
k alt+period; sleep 1.2                                               # notes drawer
ty "rate-limit the list endpoints"; k Tab
ty "add the index on lists.user_id"; k Return; sleep 0.4
k Up; sleep 0.6; k space; sleep 2.5; k Escape
sleep 1.5
k super+u; sleep 2       # next: the Pi agent that finished while I was busy
k super+b; sleep 2.2     # condense the sidebar
k super+b; sleep 1.5
k super+2; sleep 2.5     # the other project's agent is done too
kill -INT $rec; wait $rec || true

mkdir -p "$(dirname "$out")"
ffmpeg -y -loglevel error -i "$tmp/demo.mp4" -vf \
  "fps=10,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
  "$out"
"$repo/demo/nested.sh" stop
echo "wrote $out ($(du -h "$out" | cut -f1))"
