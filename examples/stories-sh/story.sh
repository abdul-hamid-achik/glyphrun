#!/bin/sh
# A dependency-free story harness: stories are black-box, so any program that
# can print escape sequences works. Usage: story.sh <story-id> | --list
set -eu

list() {
  printf '%s\n' banner/hello banner/warn
}

paint_banner_hello() {
  printf '\033[1mglyphrun\033[0m stories in plain sh\r\n'
  printf '\r\n'
  printf '  hello from a POSIX shell harness\r\n'
  printf '\r\n'
  printf '\033[2mq quit\033[0m\r\n'
}

paint_banner_warn() {
  printf '\033[1mglyphrun\033[0m stories in plain sh\r\n'
  printf '\r\n'
  printf '  \033[33mwarning:\033[0m no toolchain was harmed\r\n'
  printf '\r\n'
  printf '\033[2mq quit\033[0m\r\n'
}

case "${1:-}" in
  --list|-list) list; exit 0 ;;
  "") printf 'usage: %s <story-id> | --list\n' "$0" >&2; exit 2 ;;
esac

printf '\033[?1049h\033[2J\033[H'
trap 'printf "\033[?1049l"' EXIT

case "$1" in
  banner/hello) paint_banner_hello ;;
  banner/warn) paint_banner_warn ;;
  *) printf 'unknown story %s\n' "$1" >&2; exit 1 ;;
esac

if [ -t 0 ]; then
  stty -echo -icanon min 1 time 0 2>/dev/null || true
fi
while :; do
  key=$(dd bs=1 count=1 2>/dev/null || true)
  [ "$key" = "q" ] && break
done
