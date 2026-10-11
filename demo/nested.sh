#!/bin/bash
# nested.sh — run flock in a nested i3 (Xephyr) for testing and recording demos.
# It has its own D-Bus session, dunst, polybar, tmux server and XDG dirs, so it
# never touches the desktop you are running it from.
#
#   demo/nested.sh start    build flock and start the nested session
#   demo/nested.sh env      print the environment for driving it (eval "$(…)")
#   demo/nested.sh stop
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
work=${FLOCK_WORK:-/tmp/flock-nested}
display=${FLOCK_DISPLAY:-:5}
size=${FLOCK_SIZE:-1280x800}

env_vars() {
  cat <<EOF
export DISPLAY=$display
export XDG_RUNTIME_DIR=$work/run XDG_DATA_HOME=$work/data XDG_CONFIG_HOME=$work/config
export I3SOCK=$work/run/i3.sock
export PATH=$work/bin:\$PATH
export WINIT_X11_SCALE_FACTOR=1
unset TMUX TMUX_PANE FLOCK_SOCKET
EOF
}

start() {
  mkdir -p "$work"/{bin,run,data} "$work"/config/{flock,i3,polybar,alacritty,dunst}
  chmod 700 "$work/run"
  (cd "$repo" && go build -o "$work/bin/flock" ./cmd/flock)
  cp "$repo/demo/fakeagent" "$work/bin/"
  [[ -d $HOME/.config/alacritty ]] && cp -r "$HOME/.config/alacritty/." "$work/config/alacritty/" || true
  [[ -f $HOME/.config/dunst/dunstrc ]] && cp "$HOME/.config/dunst/dunstrc" "$work/config/dunst/" || true
  printf 'terminal = "alacritty"\n' > "$work/config/flock/config.toml"

  # the example snippet, with the daemon's log captured
  sed "s|exec_always --no-startup-id flock daemon|exec_always --no-startup-id flock daemon 2>>$work/daemon.log|" \
    "$repo/examples/i3.conf" > "$work/config/i3/flock.conf"
  cat > "$work/config/i3/config" <<EOF
set \$mod Mod4
ipc-socket $work/run/i3.sock
include $work/config/i3/flock.conf
for_window [class=".*"] border pixel 3
bindsym \$mod+Up focus up
bindsym \$mod+Down focus down
bindsym \$mod+Left focus left
bindsym \$mod+Right focus right
bindsym \$mod+Shift+Up move up
bindsym \$mod+Shift+Down move down
bindsym \$mod+Shift+Left move left
bindsym \$mod+Shift+Right move right
bindsym \$mod+h split h
bindsym \$mod+v split v
bindsym \$mod+s layout stacking
bindsym \$mod+e layout toggle split
bindsym \$mod+Shift+q kill
bindsym \$mod+1 workspace number "1:WellRead"
bindsym \$mod+2 workspace number "2:StudySmarterSite"
bindsym \$mod+3 workspace number "3:Life admin"
bindsym \$mod+4 workspace number "4:flock"
exec --no-startup-id i3-msg workspace number "1:WellRead"
EOF
  cat > "$work/config/polybar/config.ini" <<'EOF'
[bar/flock]
width = 100%
height = 26
background = #1c1f26
foreground = #f3f4f5
line-size = 2
font-0 = monospace:size=10;2
font-1 = DejaVu Sans:size=10;2
modules-left = flock
modules-right = date
padding-right = 1
wm-restack = i3
override-redirect = false

[module/flock]
type = custom/script
exec = flock bar
tail = true

[module/date]
type = internal/date
date = %a %d %b %H:%M
label-foreground = #c8ccd4
EOF
  Xephyr "$display" -screen "$size" -br -ac -noreset -no-host-grab >/dev/null 2>&1 &
  sleep 1
  ( eval "$(env_vars)"; setsid dbus-run-session -- sh -c \
      "dunst >$work/dunst.log 2>&1 & (sleep 1; polybar -c $work/config/polybar/config.ini flock >$work/polybar.log 2>&1) & exec i3 -c $work/config/i3/config" \
      >"$work/i3.log" 2>&1 & )
  for _ in $(seq 50); do [[ -S $work/run/i3.sock ]] && break; sleep 0.1; done
  echo "nested i3 on $display (logs in $work)"
}

stop() {
  tmux -L flock kill-server 2>/dev/null || true
  pkill -f "Xephyr $display" 2>/dev/null || true
  rm -rf "$work/run"
}

case ${1:-} in
  start) start ;;
  stop) stop ;;
  env) env_vars ;;
  *) echo "usage: $0 start|env|stop" >&2; exit 2 ;;
esac
