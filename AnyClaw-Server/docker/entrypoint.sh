#!/bin/sh
set -e

# First-run: neither config nor workspace exists.
# If config.json is already mounted but workspace is missing we skip onboard to
# avoid the interactive "Overwrite? (y/n)" prompt hanging in a non-TTY container.
if [ ! -d "${HOME}/.anyclaw/workspace" ] && [ ! -f "${HOME}/.anyclaw/config.json" ]; then
    anyclaw onboard
    echo ""
    echo "First-run setup complete. Skillhub CLI + skill pre-installed (domestic acceleration)."
    echo "Edit ${HOME}/.anyclaw/config.json (add your API key, etc.) then restart the container."
    exit 0
fi

# Sync built-in workspace files on every start so that image updates (new/changed
# skills, etc.) propagate into the mounted workspace volume automatically.
# User-edited files outside skills/ are never overwritten.
anyclaw sync-workspace

exec anyclaw ${@:-gateway}
