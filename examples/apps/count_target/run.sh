#!/bin/bash
# count_target: emits "ok-ok-ok" on 3 lines and idles. The
# count_cells.yml spec asserts the rune counts.
echo "ok-ok-ok"
echo "ok-ok-ok"
echo "ok-ok-ok"
# Idle so the runner sees the screen settle.
while true; do sleep 60; done
